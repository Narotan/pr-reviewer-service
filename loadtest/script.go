package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	baseURL        = "http://localhost:8080"
	rpsTarget      = 5
	testDuration   = 30 * time.Second
	numTeams       = 20
	membersPerTeam = 10
	targetTimeout  = 300 * time.Millisecond
)

var (
	client = &http.Client{Timeout: 5 * time.Second}
	wg     sync.WaitGroup
	// используем канал для ограничения скорости и имитации RPS
	rateLimiter = time.NewTicker(time.Second / time.Duration(rpsTarget))

	// метрики
	successCount   int64
	failureCount   int64
	durationTotals time.Duration
	mutex          sync.Mutex
	latencies      []time.Duration
)

type TeamMember struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

type TeamRequest struct {
	TeamName string       `json:"team_name"`
	Members  []TeamMember `json:"members"`
}

func main() {
	fmt.Println("Starting Go Load Test...")
	fmt.Printf("Target RPS: %d, Duration: %s, SLI: %s\n", rpsTarget, testDuration, targetTimeout)

	// генерация тестовых данных
	teams := generateTestData()

	startTime := time.Now()
	testEnd := startTime.Add(testDuration)

	for time.Now().Before(testEnd) {
		<-rateLimiter.C // ждем разрешения от лимитера скорости

		wg.Add(1)
		go func() {
			defer wg.Done()

			// случайный выбор команды и юзера
			teamIndex := time.Now().Nanosecond() % numTeams
			team := teams[teamIndex]

			authorIndex := time.Now().Nanosecond() % membersPerTeam
			author := team.Members[authorIndex]

			runTestScenario(team.TeamName, author.UserID)
		}()
	}

	wg.Wait()
	rateLimiter.Stop()

	// расчет и вывод метрик
	totalRequests := successCount + failureCount
	if totalRequests == 0 {
		fmt.Println("No requests were made.")
		return
	}

	p95 := calculateP95(latencies)
	successRate := float64(successCount) / float64(totalRequests) * 100
	actualRPS := float64(totalRequests) / testDuration.Seconds()

	fmt.Println("\n--- Результаты Нагрузочного Теста ---")
	fmt.Printf("Общее время выполнения: %s\n", time.Since(startTime).Truncate(time.Millisecond))
	fmt.Printf("Общее запросов (симулировано): %d\n", totalRequests)
	fmt.Printf("Фактический RPS: %.2f (Цель: %d)\n", actualRPS, rpsTarget)
	fmt.Printf("Успешность (SLI): %.2f%% (Цель: 99.9%%)\n", successRate)
	fmt.Printf("95-й перцентиль времени ответа (SLI): %s (Цель: %s)\n", p95.Truncate(time.Millisecond), targetTimeout)

	if successRate >= 99.9 && p95 <= targetTimeout {
		fmt.Println("\n ТЕСТ ПРОЙДЕН: Сервис соответствует требованиям SLI по успешности и времени ответа.")
	} else {
		fmt.Println("\n ТЕСТ ПРОВАЛЕН: Сервис не соответствует одному или обоим требованиям SLI.")
	}
}

func runTestScenario(teamName, authorID string) {
	prID := uuid.New().String()

	// Создание Pull Request и его слияние
	prPayload := map[string]string{
		"pull_request_id":   prID,
		"pull_request_name": fmt.Sprintf("Feature/%s", prID),
		"author_id":         authorID,
	}
	doRequest("POST", "/pullRequest/create", prPayload)

	mergePayload := map[string]string{
		"pull_request_id": prID,
	}
	doRequest("POST", "/pullRequest/merge", mergePayload)
}

// отправляет http-запрос
func doRequest(method, path string, payload interface{}) {
	url := baseURL + path
	var body io.Reader

	if payload != nil {
		jsonPayload, _ := json.Marshal(payload)
		body = bytes.NewBuffer(jsonPayload)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		recordMetrics(false, 0)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	isSuccess := false
	if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 400 {
		isSuccess = true
	}

	// Читаем тело, чтобы переиспользовать соединение
	if resp != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	recordMetrics(isSuccess, duration)
}

func generateTestData() []TeamRequest {
	var teams []TeamRequest
	for i := 1; i <= numTeams; i++ {
		teamName := fmt.Sprintf("team_%d", i)
		var members []TeamMember
		for j := 1; j <= membersPerTeam; j++ {

			// Мы объединяем i и j, чтобы получить уникальный номер (например, i=20, j=10 -> 2010)
			combinedID := (i * 100) + j
			// Мы форматируем этот номер в 12-значную строку с ведущими нулями
			idSuffix := fmt.Sprintf("%012d", combinedID)

			// формат 8-4-4-4-12 (всего 36 символов)
			uid := fmt.Sprintf("00000000-0000-0000-0000-%s", idSuffix)

			members = append(members, TeamMember{
				UserID:   uid,
				Username: fmt.Sprintf("User_%d-%d", i, j),
				IsActive: true,
			})
		}
		teams = append(teams, TeamRequest{TeamName: teamName, Members: members})
	}

	fmt.Println("Initializing test data (creating teams/users)...")
	for _, team := range teams {
		payload, _ := json.Marshal(team)
		resp, err := http.Post(baseURL+"/team/add", "application/json", bytes.NewBuffer(payload))
		if err != nil || (resp.StatusCode != 201 && resp.StatusCode != 400) {
			fmt.Printf("Warning: Failed to create team %s or received unexpected status %d\n", team.TeamName, resp.StatusCode)
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
	fmt.Println("Initialization complete. Running load test...")
	return teams
}

// логирование метрик
func recordMetrics(success bool, duration time.Duration) {
	mutex.Lock()
	defer mutex.Unlock()

	if success {
		successCount++
	} else {
		failureCount++
	}
	durationTotals += duration
	latencies = append(latencies, duration)
}

// расчет 95-го перцентиля
func calculateP95(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	sortDurations(latencies)

	index := int(float64(len(latencies))*0.95) - 1
	if index < 0 {
		index = 0
	}
	return latencies[index]
}

func sortDurations(arr []time.Duration) {
	sort.Slice(arr, func(i, j int) bool {
		return arr[i] < arr[j]
	})
}
