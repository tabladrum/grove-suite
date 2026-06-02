CREATE TABLE IF NOT EXISTS intents (
    id             TEXT PRIMARY KEY,
    description    TEXT NOT NULL,
    domain         TEXT NOT NULL,
    source         TEXT NOT NULL DEFAULT 'native',
    source_ref     TEXT,
    status         TEXT NOT NULL DEFAULT 'draft',
    gs_score       REAL,
    icr_symbols    INTEGER,
    author         TEXT NOT NULL,
    approved_by    TEXT,
    approved_at    TIMESTAMPTZ,
    rejected_by    TEXT,
    rejected_at    TIMESTAMPTZ,
    rejection_note TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS intent_events (
    id         BIGSERIAL PRIMARY KEY,
    intent_id  TEXT NOT NULL REFERENCES intents(id) ON DELETE CASCADE,
    type       TEXT NOT NULL,
    detail     TEXT,
    actor      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_intents_status    ON intents(status);
CREATE INDEX IF NOT EXISTS idx_intents_domain    ON intents(domain);
CREATE INDEX IF NOT EXISTS idx_intents_source    ON intents(source, source_ref);
CREATE INDEX IF NOT EXISTS idx_events_intent_id  ON intent_events(intent_id);
