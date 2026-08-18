CREATE TABLE IF NOT EXISTS terminal_events (
  id bigserial PRIMARY KEY,
  session_id text NOT NULL REFERENCES incident_sessions(id) ON DELETE CASCADE,
  command text NOT NULL,
  occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS terminal_events_session_time_idx ON terminal_events (session_id, occurred_at);

CREATE TABLE IF NOT EXISTS submissions (
  id bigserial PRIMARY KEY,
  session_id text NOT NULL UNIQUE REFERENCES incident_sessions(id) ON DELETE CASCADE,
  score integer NOT NULL CHECK (score BETWEEN 0 AND 100),
  validated boolean NOT NULL,
  submitted_at timestamptz NOT NULL DEFAULT now()
);
