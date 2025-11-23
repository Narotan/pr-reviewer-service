package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Narotan/pr-reviewer-service/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Store interface {
	CreateTeam(ctx context.Context, name string) (db.Team, error)
	GetTeamByName(ctx context.Context, name string) (db.Team, error)
	GetUsersByTeamID(ctx context.Context, teamID pgtype.UUID) ([]db.User, error)
	CreateUser(ctx context.Context, arg db.CreateUserParams) (db.User, error)
	UpsertUser(ctx context.Context, arg db.UpsertUserParams) (db.User, error)
	GetUser(ctx context.Context, id uuid.UUID) (db.User, error)
	GetTeam(ctx context.Context, id uuid.UUID) (db.Team, error)
	SetUserActive(ctx context.Context, arg db.SetUserActiveParams) error
	CreatePullRequest(ctx context.Context, arg db.CreatePullRequestParams) (db.PullRequest, error)
	CreatePullRequestWithID(ctx context.Context, arg db.CreatePullRequestWithIDParams) (db.PullRequest, error)
	UpdatePullRequestStatus(ctx context.Context, arg db.UpdatePullRequestStatusParams) (db.PullRequest, error)
	GetPullRequest(ctx context.Context, id uuid.UUID) (db.PullRequest, error)
	GetOpenPullRequestsForReviewer(ctx context.Context, userID uuid.UUID) ([]db.PullRequest, error)
	AddReviewerToPR(ctx context.Context, arg db.AddReviewerToPRParams) error
	RemoveReviewerFromPR(ctx context.Context, arg db.RemoveReviewerFromPRParams) error
	GetReviewersForPR(ctx context.Context, prID uuid.UUID) ([]db.User, error)
	GetCandidatesForInitialReview(ctx context.Context, authorID uuid.UUID) ([]db.User, error)
	GetCandidatesForReassignment(ctx context.Context, arg db.GetCandidatesForReassignmentParams) ([]db.User, error)
	// выполняет fn в транзакции; fn получает объект запросов, привязанный к tx
	ExecTx(ctx context.Context, fn func(q *db.Queries) error) error
	// статистика назначений (sqlc сгенерирует методы GetAssignmentCountsByUser/GetAssignmentCountsByPR)
	GetAssignmentCountsByUser(ctx context.Context) ([]db.GetAssignmentCountsByUserRow, error)
	GetAssignmentCountsByPR(ctx context.Context) ([]db.GetAssignmentCountsByPRRow, error)
}

type UserDetails struct {
	User     db.User
	TeamName string
}

type TeamMemberDetails struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

type TeamDetails struct {
	TeamName string              `json:"team_name"`
	Members  []TeamMemberDetails `json:"members"`
}

type PRDetails struct {
	PullRequestID     string   `json:"pull_request_id"`
	PullRequestName   string   `json:"pull_request_name"`
	AuthorID          string   `json:"author_id"`
	Status            string   `json:"status"`
	AssignedReviewers []string `json:"assigned_reviewers"`
	CreatedAt         *string  `json:"createdAt,omitempty"`
	MergedAt          *string  `json:"mergedAt,omitempty"`
	ReplacedBy        string   `json:"-"` // не входит в json ответ, используется для переназначения
}

type AssignmentStats struct {
	Users []db.GetAssignmentCountsByUserRow `json:"users"`
	PRs   []db.GetAssignmentCountsByPRRow   `json:"prs"`
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{
		store: store,
	}
}

func (s *Service) CreateTeam(ctx context.Context, name string) (db.Team, error) {
	return db.Team{}, errors.New("not implemented")
}

// создает команду с участниками
func (s *Service) CreateTeamWithMembers(ctx context.Context, teamName string, members []TeamMemberDetails) (*TeamDetails, error) {
	team, err := s.store.CreateTeam(ctx, teamName)
	if err != nil {
		return nil, err
	}

	resultDetails := &TeamDetails{
		TeamName: team.Name,
		Members:  make([]TeamMemberDetails, 0, len(members)),
	}

	for _, member := range members {
		userID, err := uuid.Parse(member.UserID)
		if err != nil {
			return nil, fmt.Errorf("invalid UUID format for user %s: %w", member.UserID, err)
		}

		uid := userID
		_, err = s.store.UpsertUser(ctx, db.UpsertUserParams{
			ID:       uid,
			Name:     member.Username,
			TeamID:   pgtype.UUID{Bytes: team.ID},
			IsActive: member.IsActive,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to upsert user %s (%s) for team %s: %w", member.Username, member.UserID, teamName, err)
		}

		resultDetails.Members = append(resultDetails.Members, member)
	}

	return resultDetails, nil
}

// получает детали команды
func (s *Service) GetTeamDetails(ctx context.Context, teamName string) (*TeamDetails, error) {
	team, err := s.store.GetTeamByName(ctx, teamName)
	if err != nil {
		return nil, err
	}

	users, err := s.store.GetUsersByTeamID(ctx, pgtype.UUID{Bytes: team.ID})
	if err != nil {
		return nil, err
	}

	members := make([]TeamMemberDetails, 0, len(users))
	for _, u := range users {
		members = append(members, TeamMemberDetails{
			UserID:   u.ID.String(),
			Username: u.Name,
			IsActive: u.IsActive,
		})
	}

	return &TeamDetails{TeamName: team.Name, Members: members}, nil
}

// устанавливает статус активности пользователя
func (s *Service) SetUserActiveStatus(ctx context.Context, userID uuid.UUID, isActive bool) (*UserDetails, error) {
	if err := s.store.SetUserActive(ctx, db.SetUserActiveParams{ID: userID, IsActive: isActive}); err != nil {
		return nil, err
	}

	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	team, err := s.store.GetTeam(ctx, user.TeamID.Bytes)
	if err != nil {
		return &UserDetails{User: user, TeamName: ""}, nil
	}

	return &UserDetails{User: user, TeamName: team.Name}, nil
}

// создает pull request
func (s *Service) CreatePullRequest(ctx context.Context, prID, title, authorIDStr string) (*PRDetails, error) {
	// все операции создания PR и назначения ревьюверов выполняются в транзакции
	var createdPR db.PullRequest
	var assigned []string

	err := s.store.ExecTx(ctx, func(txq *db.Queries) error {
		// парсим id
		prUUID, err := uuid.Parse(prID)
		if err != nil {
			return fmt.Errorf("invalid pull_request_id format: %w", err)
		}
		authorID, err := uuid.Parse(authorIDStr)
		if err != nil {
			return fmt.Errorf("invalid author_id format: %w", err)
		}

		// проверяем существование PR
		if _, err := txq.GetPullRequest(ctx, prUUID); err == nil {
			return fmt.Errorf("PR_EXISTS")
		}

		// проверяем автора
		if _, err := txq.GetUser(ctx, authorID); err != nil {
			return fmt.Errorf("author not found")
		}

		// создаём PR с указанным id
		pr, err := txq.CreatePullRequestWithID(ctx, db.CreatePullRequestWithIDParams{ID: prUUID, Title: title, AuthorID: authorID})
		if err != nil {
			return err
		}
		createdPR = pr

		// выбираем кандидатов и добавляем как ревьюверов (до 2)
		candidates, err := txq.GetCandidatesForInitialReview(ctx, authorID)
		if err != nil {
			// если нет кандидатов, продолжаем без ошибок
			candidates = []db.User{}
		}
		for i := range candidates {
			if i >= 2 {
				break
			}
			if err := txq.AddReviewerToPR(ctx, db.AddReviewerToPRParams{PrID: pr.ID, UserID: candidates[i].ID}); err != nil {
				return err
			}
			assigned = append(assigned, candidates[i].ID.String())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	createdAt := createdPR.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00")
	return &PRDetails{
		PullRequestID:     createdPR.ID.String(),
		PullRequestName:   createdPR.Title,
		AuthorID:          createdPR.AuthorID.String(),
		Status:            createdPR.Status,
		AssignedReviewers: assigned,
		CreatedAt:         &createdAt,
	}, nil
}

// обновляет статус pr на merged
func (s *Service) UpdatePRStatusToMerged(ctx context.Context, prIDStr string) (*PRDetails, error) {
	prID, err := uuid.Parse(prIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid pull_request_id format: %w", err)
	}

	existingPR, err := s.store.GetPullRequest(ctx, prID)
	if err != nil {
		return nil, fmt.Errorf("PR not found")
	}

	if existingPR.Status == "MERGED" {
		reviewers, _ := s.store.GetReviewersForPR(ctx, prID)
		reviewerIDs := make([]string, len(reviewers))
		for i, r := range reviewers {
			reviewerIDs[i] = r.ID.String()
		}

		createdAt := existingPR.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00")
		mergedAt := existingPR.UpdatedAt.Time.Format("2006-01-02T15:04:05Z07:00")

		return &PRDetails{
			PullRequestID:     existingPR.ID.String(),
			PullRequestName:   existingPR.Title,
			AuthorID:          existingPR.AuthorID.String(),
			Status:            existingPR.Status,
			AssignedReviewers: reviewerIDs,
			CreatedAt:         &createdAt,
			MergedAt:          &mergedAt,
		}, nil
	}

	pr, err := s.store.UpdatePullRequestStatus(ctx, db.UpdatePullRequestStatusParams{ID: prID, Status: "MERGED"})
	if err != nil {
		return nil, err
	}

	reviewers, _ := s.store.GetReviewersForPR(ctx, prID)
	reviewerIDs := make([]string, len(reviewers))
	for i, r := range reviewers {
		reviewerIDs[i] = r.ID.String()
	}

	createdAt := pr.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00")
	mergedAt := pr.UpdatedAt.Time.Format("2006-01-02T15:04:05Z07:00")

	return &PRDetails{
		PullRequestID:     pr.ID.String(),
		PullRequestName:   pr.Title,
		AuthorID:          pr.AuthorID.String(),
		Status:            pr.Status,
		AssignedReviewers: reviewerIDs,
		CreatedAt:         &createdAt,
		MergedAt:          &mergedAt,
	}, nil
}

type PRShort struct {
	PullRequestID   string `json:"pull_request_id"`
	PullRequestName string `json:"pull_request_name"`
	AuthorID        string `json:"author_id"`
	Status          string `json:"status"`
}

// получает открытые pr для ревьювера
func (s *Service) GetOpenPRsForReviewer(ctx context.Context, userID uuid.UUID) ([]PRShort, error) {
	prs, err := s.store.GetOpenPullRequestsForReviewer(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]PRShort, len(prs))
	for i, pr := range prs {
		result[i] = PRShort{
			PullRequestID:   pr.ID.String(),
			PullRequestName: pr.Title,
			AuthorID:        pr.AuthorID.String(),
			Status:          pr.Status,
		}
	}
	return result, nil
}

// переназначает ревьювера
func (s *Service) ReassignReviewer(ctx context.Context, prIDStr string, oldReviewerIDStr string) (*PRDetails, error) {
	prID, err := uuid.Parse(prIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid pull_request_id format: %w", err)
	}
	oldReviewerID, err := uuid.Parse(oldReviewerIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id format: %w", err)
	}

	pr, err := s.store.GetPullRequest(ctx, prID)
	if err != nil {
		return nil, fmt.Errorf("NOT_FOUND: PR not found")
	}

	if pr.Status == "MERGED" {
		return nil, fmt.Errorf("PR_MERGED: cannot reassign on merged PR")
	}

	reviewers, err := s.store.GetReviewersForPR(ctx, prID)
	if err != nil {
		return nil, err
	}

	isAssigned := false
	for _, r := range reviewers {
		if r.ID == oldReviewerID {
			isAssigned = true
			break
		}
	}

	if !isAssigned {
		return nil, fmt.Errorf("NOT_ASSIGNED: reviewer is not assigned to this PR")
	}

	candidates, err := s.store.GetCandidatesForReassignment(ctx, db.GetCandidatesForReassignmentParams{
		ID:   oldReviewerID,
		PrID: prID,
	})
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("NO_CANDIDATE: no active replacement candidate in team")
	}

	newReviewer := candidates[0]
	newReviewerID := newReviewer.ID

	if err := s.store.RemoveReviewerFromPR(ctx, db.RemoveReviewerFromPRParams{
		PrID:   prID,
		UserID: oldReviewerID,
	}); err != nil {
		return nil, err
	}

	if err := s.store.AddReviewerToPR(ctx, db.AddReviewerToPRParams{
		PrID:   prID,
		UserID: newReviewerID,
	}); err != nil {
		return nil, err
	}

	updatedPR, err := s.store.GetPullRequest(ctx, prID)
	if err != nil {
		return nil, err
	}

	updatedReviewers, _ := s.store.GetReviewersForPR(ctx, prID)
	reviewerIDs := make([]string, len(updatedReviewers))
	for i, r := range updatedReviewers {
		reviewerIDs[i] = r.ID.String()
	}

	createdAt := updatedPR.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00")

	details := &PRDetails{
		PullRequestID:     updatedPR.ID.String(),
		PullRequestName:   updatedPR.Title,
		AuthorID:          updatedPR.AuthorID.String(),
		Status:            updatedPR.Status,
		AssignedReviewers: reviewerIDs,
		CreatedAt:         &createdAt,
		ReplacedBy:        newReviewerID.String(),
	}

	return details, nil
}

func (s *Service) GetAssignmentStats(ctx context.Context) (*AssignmentStats, error) {
	users, err := s.store.GetAssignmentCountsByUser(ctx)
	if err != nil {
		return nil, err
	}
	prs, err := s.store.GetAssignmentCountsByPR(ctx)
	if err != nil {
		return nil, err
	}
	// Ensure we return empty slices instead of null in JSON
	if users == nil {
		users = make([]db.GetAssignmentCountsByUserRow, 0)
	}
	if prs == nil {
		prs = make([]db.GetAssignmentCountsByPRRow, 0)
	}
	return &AssignmentStats{Users: users, PRs: prs}, nil
}
