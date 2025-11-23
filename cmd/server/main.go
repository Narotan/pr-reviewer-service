package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Narotan/pr-reviewer-service/internal/config"
	"github.com/Narotan/pr-reviewer-service/internal/db"
	"github.com/Narotan/pr-reviewer-service/internal/handler"
	"github.com/Narotan/pr-reviewer-service/internal/service"
)

func main() {
	// загрузка конфига
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}

	// настройка логгера
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Warn().Msgf("unknown log level %s", cfg.LogLevel)
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	log.Info().Msgf("starting pr-reviewer-service on port %s", cfg.Port)

	// подключение к postgres
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	// проверка жива ли база данных
	if err := pool.Ping(ctx); err != nil {
		log.Fatal().Err(err).Msg("database ping failed")
	}
	log.Info().Msg("database connection established")

	// инициализация слоев приложения
	queries := db.New(pool)
	svc := service.NewService(queries)
	h := handler.NewHandler(svc, &log.Logger)

	// настройка HTTP сервера
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	// --- Teams ---
	r.Post("/team/add", h.CreateTeamWithMembers)
	r.Get("/team/get", h.GetTeam)

	// --- Users  ---
	r.Post("/users/setIsActive", h.SetUserActiveStatus)
	r.Get("/users/getReview", h.GetPRsForUser)

	// --- Pull Requests ---
	r.Post("/pullRequest/create", h.CreatePullRequest)
	r.Post("/pullRequest/merge", h.MergePullRequest)
	r.Post("/pullRequest/reassign", h.ReassignReviewer)

	// --- Stats ---
	r.Get("/stats/assignments", h.GetAssignmentStats)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		log.Info().Msg("hello, world endpoint was called")
		if _, err := w.Write([]byte("hello,world! db connection is ok.")); err != nil {
			log.Warn().Err(err).Msg("failed to write response")
		}

	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Port),
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed to start")
		}
	}()
	log.Info().Msg("service is listening")

	// ожидание сигнала завершения, чтоб завершить работу корректно
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Info().Msgf("received signal: %s. shutting down...", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal().Err(err).Msg("server shutdown failed")
	}

	log.Info().Msg("server shutdown complete")
}
