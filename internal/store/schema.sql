CREATE TABLE IF NOT EXISTS nodes (
  node_id     TEXT PRIMARY KEY,
  peer_id     TEXT UNIQUE,
  bind_tty    TEXT,
  name        TEXT NOT NULL,
  role        TEXT NOT NULL,
  parent_id   TEXT REFERENCES nodes(node_id),
  workdir     TEXT NOT NULL,
  spawn_mode  TEXT NOT NULL,
  spawn_ref   TEXT,
  state       TEXT NOT NULL CHECK (state IN ('pending','alive','dead','failed')),
  created_at  TEXT NOT NULL,
  died_at     TEXT
);

CREATE TABLE IF NOT EXISTS roles (
  role   TEXT PRIMARY KEY,
  glyph  TEXT NOT NULL,
  prompt TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS gates (
  node_id      TEXT NOT NULL REFERENCES nodes(node_id),
  allowed_from TEXT NOT NULL
);
