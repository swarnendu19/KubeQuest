CREATE TABLE IF NOT EXISTS session_attempts (
  id bigserial PRIMARY KEY,
  session_id text NOT NULL REFERENCES incident_sessions(id) ON DELETE CASCADE,
  attempt_number integer NOT NULL,
  started_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (session_id, attempt_number)
);

CREATE TABLE IF NOT EXISTS audit_events (
  id bigserial PRIMARY KEY,
  session_id text REFERENCES incident_sessions(id) ON DELETE SET NULL,
  event_type text NOT NULL,
  occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_events_session_time_idx ON audit_events (session_id, occurred_at);
