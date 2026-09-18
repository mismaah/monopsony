CREATE TABLE IF NOT EXISTS users (
  id            TEXT PRIMARY KEY,
  email         TEXT UNIQUE,
  name          TEXT NOT NULL,
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'player',
  tier          TEXT NOT NULL DEFAULT 'free',
  guest         BOOLEAN NOT NULL DEFAULT FALSE,
  banned        BOOLEAN NOT NULL DEFAULT FALSE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
  hash       TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS refresh_tokens_user ON refresh_tokens(user_id);

CREATE TABLE IF NOT EXISTS game_configs (
  id         TEXT PRIMARY KEY,
  version    INTEGER NOT NULL,
  name       TEXT NOT NULL,
  config     JSONB NOT NULL,
  published  BOOLEAN NOT NULL DEFAULT FALSE,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS game_configs_one_published ON game_configs(published) WHERE published;

CREATE TABLE IF NOT EXISTS games (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL,
  status       TEXT NOT NULL,
  host_id      TEXT NOT NULL,
  visibility   TEXT NOT NULL,
  invite_code  TEXT,
  max_players  INTEGER NOT NULL,
  turn_seconds INTEGER NOT NULL,
  config_id    TEXT NOT NULL,
  config       JSONB NOT NULL,
  seats        JSONB NOT NULL,
  state        JSONB,
  seq          INTEGER NOT NULL DEFAULT 0,
  winner_id    TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at   TIMESTAMPTZ,
  finished_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS games_status ON games(status, visibility, created_at DESC);
CREATE INDEX IF NOT EXISTS games_seats ON games USING GIN (seats jsonb_path_ops);

CREATE TABLE IF NOT EXISTS game_events (
  game_id TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  seq     INTEGER NOT NULL,
  type    TEXT NOT NULL,
  payload JSONB NOT NULL,
  PRIMARY KEY (game_id, seq)
);
