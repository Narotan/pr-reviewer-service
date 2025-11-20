DROP TABLE IF EXISTS pr_reviewers;
DROP TABLE IF EXISTS pull_requests;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS teams;

-- Удаляем наш кастомный тип
DROP TYPE IF EXISTS pr_status;