-- расшириение для генерации UUID
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- кастомный тип для статуса Pull Request
CREATE TYPE pr_status AS ENUM ('OPEN', 'MERGED');

CREATE TABLE teams (
    id   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE users (
    id        UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name      TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    -- связываем пользователя с командой
    team_id   UUID REFERENCES teams(id) ON DELETE SET NULL
);

CREATE TABLE pull_requests (
    id         UUID      PRIMARY KEY DEFAULT uuid_generate_v4(),
    title      TEXT      NOT NULL,

    -- внешний ключ: связываем PR с автором
    author_id  UUID      NOT NULL REFERENCES users(id),

    status     pr_status NOT NULL DEFAULT 'OPEN',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE pr_reviewers (
    pr_id  UUID NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- что бы один и тот же пользователь не мог быть добавлен несколько раз к одному PR
    PRIMARY KEY (pr_id, user_id)
);