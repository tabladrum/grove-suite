CREATE TABLE IF NOT EXISTS repos (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL UNIQUE,
    url            TEXT NOT NULL UNIQUE,
    default_branch TEXT NOT NULL DEFAULT 'main',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS projects (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    repo_id       TEXT NOT NULL REFERENCES repos(id),
    source_path   TEXT NOT NULL DEFAULT '/',
    gs_threshold  REAL NOT NULL DEFAULT 0.70,
    auto_approve  BOOLEAN NOT NULL DEFAULT FALSE,
    owner         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS project_integrations (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,
    external_id TEXT NOT NULL,
    config      JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, type, external_id)
);

ALTER TABLE intents ADD COLUMN IF NOT EXISTS project_id TEXT REFERENCES projects(id);
ALTER TABLE intents ALTER COLUMN domain DROP NOT NULL;
ALTER TABLE intents ALTER COLUMN domain SET DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_intents_project_id            ON intents(project_id);
CREATE INDEX IF NOT EXISTS idx_intents_unrouted              ON intents(status) WHERE status = 'unrouted';
CREATE INDEX IF NOT EXISTS idx_projects_repo_id              ON projects(repo_id);
CREATE INDEX IF NOT EXISTS idx_integrations_type_external    ON project_integrations(type, external_id);
