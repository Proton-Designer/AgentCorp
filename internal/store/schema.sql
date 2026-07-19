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
  died_at     TEXT,
  session_id  TEXT
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

-- Supervision (self-healing): restart history is an audit trail and must
-- outlive a fired/disbanded node, so it deliberately has no FK to nodes --
-- same reasoning as persistence-design.md's proposed events table.
CREATE TABLE IF NOT EXISTS restarts (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  node_id TEXT NOT NULL,
  at      TEXT NOT NULL,
  reason  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS restarts_node_id ON restarts(node_id);

-- supervision_policy IS FK'd to nodes: it's live configuration that only
-- means something while the node exists, not an audit record. Absence of a
-- row means "use the system default" -- every hire/adopt flow is unaffected.
CREATE TABLE IF NOT EXISTS supervision_policy (
  node_id        TEXT PRIMARY KEY REFERENCES nodes(node_id),
  strategy       TEXT NOT NULL DEFAULT 'one-for-one'
                   CHECK (strategy IN ('one-for-one','one-for-all','rest-for-one')),
  max_restarts   INTEGER NOT NULL DEFAULT 3,
  window_seconds INTEGER NOT NULL DEFAULT 300
);
