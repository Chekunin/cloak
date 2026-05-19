package store

const schemaSQL = `
CREATE TABLE IF NOT EXISTS secrets (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL UNIQUE,
    type              TEXT NOT NULL,
    description       TEXT,
    config_json       TEXT NOT NULL,
    secret_blob       BLOB NOT NULL,
    endpoint_config   TEXT NOT NULL,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    last_used_at      INTEGER
);

CREATE INDEX IF NOT EXISTS idx_secrets_name ON secrets(name);
CREATE INDEX IF NOT EXISTS idx_secrets_type ON secrets(type);

CREATE TABLE IF NOT EXISTS client_tokens (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    token_hash   BLOB NOT NULL,
    token_salt   BLOB NOT NULL,
    created_at   INTEGER NOT NULL,
    last_seen_at INTEGER,
    revoked      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
