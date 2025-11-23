package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/Narotan/pr-reviewer-service/internal/db"
	"github.com/Narotan/pr-reviewer-service/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"
)

type Service interface {
	CreateTeam(ctx context.Context, name string) (db.Team, error)
	CreateTeamWithMembers(ctx context.Context, teamName string, members []service.TeamMemberDetails) (*service.TeamDetails, error)
	GetTeamDetails(ctx context.Context, teamName string) (*service.TeamDetails, error)
	SetUserActiveStatus(ctx context.Context, userID uuid.UUID, isActive bool) (*service.UserDetails, error)
	CreatePullRequest(ctx context.Context, prID, title, authorID string) (*service.PRDetails, error)
	UpdatePRStatusToMerged(ctx context.Context, prID string) (*service.PRDetails, error)
	GetOpenPRsForReviewer(ctx context.Context, userID uuid.UUID) ([]service.PRShort, error)
	ReassignReviewer(ctx context.Context, prID string, oldReviewerID string) (*service.PRDetails, error)
	// статистика
	GetAssignmentStats(ctx context.Context) (*service.AssignmentStats, error)
}

type Handler struct {
	service Service
	log     *zerolog.Logger
}

// создание handler
func NewHandler(service Service, log *zerolog.Logger) *Handler {
	return &Handler{
		service: service,
		log:     log,
	}
}

// структура запроса для команды
type TeamRequest struct {
	TeamName string                      `json:"team_name"`
	Members  []service.TeamMemberDetails `json:"members"`
}

// структура ответа для команды
type TeamResponse struct {
	TeamName string                      `json:"team_name"`
	Members  []service.TeamMemberDetails `json:"members"`
}

// структура ответа для пулл-реквеста
type PullRequestResponse struct {
	PullRequestID     string   `json:"pull_request_id"`
	PullRequestName   string   `json:"pull_request_name"`
	AuthorID          string   `json:"author_id"`
	Status            string   `json:"status"`
	AssignedReviewers []string `json:"assigned_reviewers"`
	CreatedAt         *string  `json:"createdAt,omitempty"`
	MergedAt          *string  `json:"mergedAt,omitempty"`
}

// структура запроса для активации/деактивации пользователя
type SetUserActiveRequest struct {
	UserID   string `json:"user_id"`
	IsActive bool   `json:"is_active"`
}

// структура запроса для создания пулл-реквеста
type PullRequestRequest struct {
	PullRequestID   string `json:"pull_request_id"`
	PullRequestName string `json:"pull_request_name"`
	AuthorID        string `json:"author_id"`
}

// структура короткого описания пулл-реквеста
type PullRequestShort struct {
	PullRequestID   string `json:"pull_request_id"`
	PullRequestName string `json:"pull_request_name"`
	AuthorID        string `json:"author_id"`
	Status          string `json:"status"`
}

func (h *Handler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	respondWithError(w, h.log, http.StatusNotImplemented, "NOT_FOUND", "Endpoint POST /teams is deprecated. Use /team/add.")
}

func (h *Handler) CreateTeamWithMembers(w http.ResponseWriter, r *http.Request) {
	var req TeamRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "invalid request format")
		return
	}

	req.TeamName = strings.TrimSpace(req.TeamName)
	if req.TeamName == "" {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "team_name cannot be empty")
		return
	}

	if len(req.Members) == 0 {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "members list cannot be empty")
		return
	}

	for _, member := range req.Members {
		if _, err := uuid.Parse(member.UserID); err != nil {
			respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "invalid user_id format (must be UUID)")
			return
		}
		if strings.TrimSpace(member.Username) == "" {
			respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "username cannot be empty")
			return
		}
	}

	teamDetails, err := h.service.CreateTeamWithMembers(r.Context(), req.TeamName, req.Members)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			respondWithError(w, h.log, http.StatusBadRequest, "TEAM_EXISTS", "team_name already exists")
			return
		}
		h.log.Error().Err(err).Msg("failed to create team with members")
		respondWithError(w, h.log, http.StatusInternalServerError, "INTERNAL", "internal server error")
		return
	}

	response := map[string]interface{}{
		"team": TeamResponse{
			TeamName: teamDetails.TeamName,
			Members:  teamDetails.Members,
		},
	}
	respondWithJSON(w, h.log, http.StatusCreated, response)
}

func (h *Handler) GetTeam(w http.ResponseWriter, r *http.Request) {
	// query param team_name
	q := r.URL.Query().Get("team_name")
	if strings.TrimSpace(q) == "" {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "team_name query missing")
		return
	}
	td, err := h.service.GetTeamDetails(r.Context(), q)
	if err != nil {
		respondWithError(w, h.log, http.StatusNotFound, "NOT_FOUND", "team not found")
		return
	}
	respondWithJSON(w, h.log, http.StatusOK, td)
}

func (h *Handler) SetUserActiveStatus(w http.ResponseWriter, r *http.Request) {
	var req SetUserActiveRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "invalid request format")
		return
	}
	uid, err := uuid.Parse(req.UserID)
	if err != nil {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "invalid user_id format")
		return
	}
	userDetails, err := h.service.SetUserActiveStatus(r.Context(), uid, req.IsActive)
	if err != nil {
		respondWithError(w, h.log, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}
	respondWithJSON(w, h.log, http.StatusOK, map[string]interface{}{"user": userDetails})
}

func (h *Handler) GetPRsForUser(w http.ResponseWriter, r *http.Request) {
	uidq := r.URL.Query().Get("user_id")
	if uidq == "" {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "user_id query missing")
		return
	}
	uid, err := uuid.Parse(uidq)
	if err != nil {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "invalid user_id format")
		return
	}
	prs, err := h.service.GetOpenPRsForReviewer(r.Context(), uid)
	if err != nil {
		h.log.Error().Err(err).Msg("failed to get PRs for user")
		respondWithError(w, h.log, http.StatusInternalServerError, "INTERNAL", "internal server error")
		return
	}
	respondWithJSON(w, h.log, http.StatusOK, map[string]interface{}{
		"user_id":       uidq,
		"pull_requests": prs,
	})
}

func (h *Handler) CreatePullRequest(w http.ResponseWriter, r *http.Request) {
	var req PullRequestRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "invalid request format")
		return
	}

	if strings.TrimSpace(req.PullRequestID) == "" {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "pull_request_id is required")
		return
	}
	if strings.TrimSpace(req.PullRequestName) == "" {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "pull_request_name is required")
		return
	}
	if strings.TrimSpace(req.AuthorID) == "" {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "author_id is required")
		return
	}

	prDetails, err := h.service.CreatePullRequest(r.Context(), req.PullRequestID, req.PullRequestName, req.AuthorID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "PR_EXISTS") {
			respondWithError(w, h.log, http.StatusConflict, "PR_EXISTS", "PR id already exists")
			return
		}
		if strings.Contains(errMsg, "not found") {
			respondWithError(w, h.log, http.StatusNotFound, "NOT_FOUND", "author not found")
			return
		}
		h.log.Error().Err(err).Msg("failed to create pull request")
		respondWithError(w, h.log, http.StatusInternalServerError, "INTERNAL", "internal server error")
		return
	}

	respondWithJSON(w, h.log, http.StatusCreated, map[string]interface{}{"pr": prDetails})
}

func (h *Handler) MergePullRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PullRequestID string `json:"pull_request_id"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "invalid request format")
		return
	}

	if strings.TrimSpace(req.PullRequestID) == "" {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "pull_request_id is required")
		return
	}

	prDetails, err := h.service.UpdatePRStatusToMerged(r.Context(), req.PullRequestID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondWithError(w, h.log, http.StatusNotFound, "NOT_FOUND", "PR not found")
			return
		}
		h.log.Error().Err(err).Msg("failed to merge pull request")
		respondWithError(w, h.log, http.StatusInternalServerError, "INTERNAL", "internal server error")
		return
	}

	respondWithJSON(w, h.log, http.StatusOK, map[string]interface{}{"pr": prDetails})
}

func (h *Handler) ReassignReviewer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PullRequestID string `json:"pull_request_id"`
		OldUserID     string `json:"old_user_id"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "invalid request format")
		return
	}

	if strings.TrimSpace(req.PullRequestID) == "" {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "pull_request_id is required")
		return
	}
	if strings.TrimSpace(req.OldUserID) == "" {
		respondWithError(w, h.log, http.StatusBadRequest, "BAD_REQUEST", "old_user_id is required")
		return
	}

	prDetails, err := h.service.ReassignReviewer(r.Context(), req.PullRequestID, req.OldUserID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "NOT_FOUND") {
			respondWithError(w, h.log, http.StatusNotFound, "NOT_FOUND", "PR or user not found")
			return
		}
		if strings.Contains(errMsg, "PR_MERGED") {
			respondWithError(w, h.log, http.StatusConflict, "PR_MERGED", "cannot reassign on merged PR")
			return
		}
		if strings.Contains(errMsg, "NOT_ASSIGNED") {
			respondWithError(w, h.log, http.StatusConflict, "NOT_ASSIGNED", "reviewer is not assigned to this PR")
			return
		}
		if strings.Contains(errMsg, "NO_CANDIDATE") {
			respondWithError(w, h.log, http.StatusConflict, "NO_CANDIDATE", "no active replacement candidate in team")
			return
		}
		h.log.Error().Err(err).Msg("failed to reassign reviewer")
		respondWithError(w, h.log, http.StatusInternalServerError, "INTERNAL", "internal server error")
		return
	}

	respondWithJSON(w, h.log, http.StatusOK, map[string]interface{}{
		"pr":          prDetails,
		"replaced_by": prDetails.ReplacedBy,
	})
}

// GetAssignmentStats возвращает статистику назначений (по пользователям и по PR)
func (h *Handler) GetAssignmentStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.GetAssignmentStats(r.Context())
	if err != nil {
		h.log.Error().Err(err).Msg("failed to get assignment stats")
		respondWithError(w, h.log, http.StatusInternalServerError, "INTERNAL", "internal server error")
		return
	}
	respondWithJSON(w, h.log, http.StatusOK, map[string]interface{}{"stats": stats})
}
