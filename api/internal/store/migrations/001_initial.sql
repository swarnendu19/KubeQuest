CREATE TABLE IF NOT EXISTS users (
  id text PRIMARY KEY,
  email text UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS scenarios (
  id text PRIMARY KEY,
  title text NOT NULL,
  difficulty text NOT NULL,
  duration_min integer NOT NULL CHECK (duration_min > 0),
  technologies jsonb NOT NULL DEFAULT '[]'::jsonb,
  definition jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS incident_sessions (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id),
  scenario_id text NOT NULL REFERENCES scenarios(id),
  state text NOT NULL CHECK (state IN ('creating','provisioning','ready','running','resetting','completed','expired','destroyed')),
  runtime_id text,
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS incident_sessions_user_state_idx ON incident_sessions (user_id, state);

CREATE TABLE IF NOT EXISTS incident_events (
  id bigserial PRIMARY KEY,
  session_id text NOT NULL REFERENCES incident_sessions(id) ON DELETE CASCADE,
  type text NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS incident_events_session_time_idx ON incident_events (session_id, occurred_at);

INSERT INTO users (id, email) VALUES ('dev-user', 'dev@kernelquest.local') ON CONFLICT (id) DO NOTHING;
INSERT INTO scenarios (id, title, difficulty, duration_min, technologies) VALUES
  ('broken-nginx-upstream', 'Broken Nginx Upstream', 'Beginner', 15, '["Linux","Nginx","HTTP"]'),
  ('disk-exhaustion', 'Disk Exhaustion', 'Beginner', 18, '["Linux","Storage","Logs"]'),
  ('postgres-pool-exhaustion', 'Checkout API Suddenly Became Slow', 'Intermediate', 20, '["PostgreSQL","Nginx","Redis"]')
ON CONFLICT (id) DO NOTHING;
