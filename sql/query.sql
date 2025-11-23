-- --- Команды ---

-- name: CreateTeam :one
INSERT INTO teams (name)
VALUES ($1)
RETURNING *;

-- name: GetTeam :one
SELECT * FROM teams
WHERE id = $1;

-- name: GetTeamByName :one
SELECT * FROM teams
WHERE name = $1;

-- --- Пользователи ---

-- name: CreateUser :one
INSERT INTO users (name, team_id)
VALUES ($1, $2)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users
WHERE id = $1;

-- name: SetUserActive :exec
UPDATE users
SET is_active = $2
WHERE id = $1;

-- name: DeactivateUsersByTeam :exec
UPDATE users
SET is_active = false
WHERE team_id = $1 AND is_active = true;

-- name: UpsertUser :one
INSERT INTO users (id, name, team_id, is_active)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE
    SET
    name = EXCLUDED.name,
    team_id = EXCLUDED.team_id,
    is_active = EXCLUDED.is_active
RETURNING *;

-- name: GetUsersByTeamID :many
SELECT * FROM users
WHERE team_id = $1;

-- --- Пулл-реквесты ---

-- name: CreatePullRequest :one
INSERT INTO pull_requests (title, author_id)
VALUES ($1, $2)
RETURNING *;

-- name: CreatePullRequestWithID :one
INSERT INTO pull_requests (id, title, author_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPullRequest :one
SELECT * FROM pull_requests
WHERE id = $1;

-- name: UpdatePullRequestStatus :one
UPDATE pull_requests
SET status = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetOpenPullRequestsForReviewer :many
SELECT pr.*
FROM pull_requests pr
JOIN pr_reviewers prr ON pr.id = prr.pr_id
WHERE prr.user_id = $1 AND pr.status = 'OPEN';

-- --- Ревьюверы ---

-- name: AddReviewerToPR :exec
INSERT INTO pr_reviewers (pr_id, user_id)
VALUES ($1, $2)
ON CONFLICT (pr_id, user_id) DO NOTHING;

-- name: RemoveReviewerFromPR :exec
DELETE FROM pr_reviewers
WHERE pr_id = $1 AND user_id = $2;

-- name: GetReviewersForPR :many
SELECT users.*
FROM users
JOIN pr_reviewers ON users.id = pr_reviewers.user_id
WHERE pr_reviewers.pr_id = $1;

-- name: GetReviewerCountForPR :one
SELECT count(*) FROM pr_reviewers
WHERE pr_id = $1;

-- --- Кандидаты на ревью ---

-- name: GetCandidatesForInitialReview :many
SELECT * FROM users u1
WHERE team_id = (SELECT team_id FROM users u2 WHERE u2.id = $1)
  AND u1.is_active = true
  AND u1.id != $1
ORDER BY random()
LIMIT 2;

-- name: GetCandidatesForReassignment :many
SELECT id, name, is_active, team_id FROM users u1
WHERE team_id = (SELECT team_id FROM users u2 WHERE u2.id = $1)
  AND u1.is_active = true
  AND u1.id != $1
  AND u1.id NOT IN (
        SELECT user_id FROM pr_reviewers WHERE pr_id = $2
  )
LIMIT 5;

-- --- Статистика назначений ---

-- name: GetAssignmentCountsByUser :many
SELECT user_id, COUNT(*) AS cnt
FROM pr_reviewers
GROUP BY user_id;

-- name: GetAssignmentCountsByPR :many
SELECT pr_id, COUNT(*) AS cnt
FROM pr_reviewers
GROUP BY pr_id;
