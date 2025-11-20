-- --- Teams ---

-- name: CreateTeam :one
INSERT INTO teams (name)
VALUES ($1)
    RETURNING *;

-- name: GetTeam :one
SELECT * FROM teams
WHERE id = $1;

-- --- Users ---

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

-- --- Pull Requests ---

-- name: CreatePullRequest :one
INSERT INTO pull_requests (title, author_id)
VALUES ($1, $2)
    RETURNING *;

-- name: GetPullRequest :one
SELECT * FROM pull_requests
WHERE id = $1;

-- name: UpdatePullRequestStatus :one
UPDATE pull_requests
SET status = $2, updated_at = NOW()
WHERE id = $1
    RETURNING *; -- Возвращаем обновленный PR

-- name: GetOpenPullRequestsForReviewer :many
SELECT pr.*
FROM pull_requests pr
         JOIN pr_reviewers prr ON pr.id = prr.pr_id
WHERE prr.user_id = $1 AND pr.status = 'OPEN';


-- --- Reviewers ---

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


-- --- основные запросы ---

-- name: GetCandidatesForInitialReview :many
SELECT * FROM users u1
WHERE
    team_id = (SELECT team_id FROM users u2 WHERE u2.id = $1)
  AND u1.is_active = true
  AND u1.id != $1
LIMIT 2;

-- name: GetCandidatesForReassignment :many
SELECT * FROM users u1
WHERE
    team_id = (SELECT team_id FROM users u2 WHERE u2.id = $1)
  AND u1.is_active = true
  AND u1.id != $2
    AND u1.id NOT IN (
        SELECT)
LIMIT 5;