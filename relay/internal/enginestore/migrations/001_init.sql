-- 001 - Engine store schema (laptop/SQLite).
-- Mirrors Postgres types where practical; uses TEXT for timestamps (RFC3339)
-- to keep the schema portable between SQLite and Postgres adapters.

CREATE TABLE IF NOT EXISTS changesets (
    id              TEXT PRIMARY KEY,
    intent_id       TEXT NOT NULL,
    intent_brief    TEXT NOT NULL DEFAULT '',
    repo_root       TEXT NOT NULL,
    base_sha        TEXT NOT NULL,
    diff            TEXT NOT NULL,
    author          TEXT NOT NULL DEFAULT '',
    agent_model     TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_changesets_intent ON changesets(intent_id);
CREATE INDEX IF NOT EXISTS idx_changesets_status ON changesets(status);

CREATE TABLE IF NOT EXISTS certificates (
    id                    TEXT PRIMARY KEY,
    changeset_id          TEXT NOT NULL,
    intent_id             TEXT NOT NULL,
    admitted_commit_sha   TEXT NOT NULL,
    base_sha              TEXT NOT NULL,
    icr_json              TEXT NOT NULL,
    policies_json         TEXT NOT NULL,
    effective_config_hash TEXT NOT NULL,
    policy_version        TEXT NOT NULL DEFAULT '',
    toolchain_image       TEXT NOT NULL DEFAULT '',
    signed_by             TEXT NOT NULL,
    signature             BLOB NOT NULL,
    created_at            TEXT NOT NULL,
    FOREIGN KEY (changeset_id) REFERENCES changesets(id)
);

CREATE INDEX IF NOT EXISTS idx_certificates_changeset ON certificates(changeset_id);
CREATE INDEX IF NOT EXISTS idx_certificates_commit ON certificates(admitted_commit_sha);
