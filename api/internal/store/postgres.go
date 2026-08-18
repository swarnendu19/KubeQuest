package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/swarnendu-maity/kernelquest/api/internal/domain"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgres(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() { s.pool.Close() }

func (s *PostgresStore) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		version := entry.Name()
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sql, err := migrationFiles.ReadFile("migrations/" + version)
		if err != nil {
			return err
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(sql)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", version, err)
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) Create(session domain.Session) error {
	_, err := s.pool.Exec(context.Background(), `INSERT INTO incident_sessions (id,user_id,scenario_id,state,runtime_id,created_at,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, session.ID, session.UserID, session.ScenarioID, session.State, session.RuntimeID, session.CreatedAt, session.ExpiresAt)
	return err
}

func (s *PostgresStore) GetOwned(id, userID string) (domain.Session, error) {
	return scanSession(s.pool.QueryRow(context.Background(), sessionQuery+` WHERE id=$1 AND user_id=$2`, id, userID))
}

func (s *PostgresStore) UpdateOwned(id, userID string, update func(*domain.Session) error) (domain.Session, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	defer tx.Rollback(ctx)
	session, err := scanSession(tx.QueryRow(ctx, sessionQuery+` WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, userID))
	if err != nil {
		return domain.Session{}, err
	}
	if err := update(&session); err != nil {
		return domain.Session{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE incident_sessions SET state=$1,runtime_id=$2 WHERE id=$3`, session.State, nullable(session.RuntimeID), session.ID)
	if err != nil {
		return domain.Session{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (s *PostgresStore) ExpireBefore(now time.Time) []domain.Session {
	rows, err := s.pool.Query(context.Background(), `UPDATE incident_sessions SET state='expired' WHERE state='running' AND expires_at < $1 RETURNING id,user_id,scenario_id,state,COALESCE(runtime_id,''),created_at,expires_at`, now)
	if err != nil {
		return nil
	}
	defer rows.Close()
	expired := []domain.Session{}
	for rows.Next() {
		var session domain.Session
		if rows.Scan(&session.ID, &session.UserID, &session.ScenarioID, &session.State, &session.RuntimeID, &session.CreatedAt, &session.ExpiresAt) == nil {
			expired = append(expired, session)
		}
	}
	return expired
}

func (s *PostgresStore) RecordSubmission(sessionID string, score int) error {
	_, err := s.pool.Exec(context.Background(), `INSERT INTO submissions (session_id, score, validated) VALUES ($1,$2,true) ON CONFLICT (session_id) DO UPDATE SET score=EXCLUDED.score, validated=true, submitted_at=now()`, sessionID, score)
	return err
}

func (s *PostgresStore) RecordCommand(sessionID, command string) error {
	_, err := s.pool.Exec(context.Background(), `INSERT INTO terminal_events (session_id,command) VALUES ($1,$2)`, sessionID, command)
	return err
}
func (s *PostgresStore) RecordAudit(sessionID, eventType string) error {
	_, err := s.pool.Exec(context.Background(), `INSERT INTO audit_events (session_id,event_type) VALUES ($1,$2)`, sessionID, eventType)
	return err
}
func (s *PostgresStore) RecordAttempt(sessionID string) error {
	_, err := s.pool.Exec(context.Background(), `INSERT INTO session_attempts (session_id,attempt_number) SELECT $1,COALESCE(MAX(attempt_number),0)+1 FROM session_attempts WHERE session_id=$1`, sessionID)
	return err
}
func (s *PostgresStore) AuditTimeline(sessionID string) ([]map[string]any,error) { rows,err:=s.pool.Query(context.Background(),`SELECT event_type,occurred_at FROM audit_events WHERE session_id=$1 ORDER BY occurred_at`,sessionID);if err!=nil{return nil,err};defer rows.Close();events:=[]map[string]any{};for rows.Next(){var kind string;var at time.Time;if err:=rows.Scan(&kind,&at);err!=nil{return nil,err};events=append(events,map[string]any{"type":kind,"occurredAt":at})};return events,rows.Err() }
func (s *PostgresStore) Attempts(sessionID string) ([]map[string]any,error) { rows,err:=s.pool.Query(context.Background(),`SELECT attempt_number,started_at FROM session_attempts WHERE session_id=$1 ORDER BY attempt_number`,sessionID);if err!=nil{return nil,err};defer rows.Close();attempts:=[]map[string]any{};for rows.Next(){var number int;var at time.Time;if err:=rows.Scan(&number,&at);err!=nil{return nil,err};attempts=append(attempts,map[string]any{"number":number,"startedAt":at})};return attempts,rows.Err() }
func (s *PostgresStore) Timeline(sessionID string) ([]map[string]any, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT command,occurred_at FROM terminal_events WHERE session_id=$1 ORDER BY occurred_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []map[string]any{}
	for rows.Next() {
		var command string
		var at time.Time
		if err := rows.Scan(&command, &at); err != nil {
			return nil, err
		}
		events = append(events, map[string]any{"command": command, "occurredAt": at})
	}
	return events, rows.Err()
}

const sessionQuery = `SELECT id,user_id,scenario_id,state,COALESCE(runtime_id,''),created_at,expires_at FROM incident_sessions`

type row interface{ Scan(...any) error }

func scanSession(row row) (domain.Session, error) {
	var session domain.Session
	err := row.Scan(&session.ID, &session.UserID, &session.ScenarioID, &session.State, &session.RuntimeID, &session.CreatedAt, &session.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, ErrNotFound
	}
	return session, err
}
func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
