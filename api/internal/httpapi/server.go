package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/gorilla/websocket"
	"github.com/swarnendu-maity/kernelquest/api/internal/domain"
	"github.com/swarnendu-maity/kernelquest/api/internal/runtime"
	"github.com/swarnendu-maity/kernelquest/api/internal/store"
	"github.com/swarnendu-maity/kernelquest/api/internal/validation"
)

type Server struct {
	store     store.Repository
	runtime   runtime.Manager
	log       *slog.Logger
	scenarios []domain.Scenario
}

func New(s store.Repository, r runtime.Manager, log *slog.Logger) *Server {
	return &Server{store: s, runtime: r, log: log, scenarios: []domain.Scenario{
		{ID: "broken-nginx-upstream", Title: "Broken Nginx Upstream", Difficulty: "Beginner", DurationMin: 15, Technologies: []string{"Linux", "Nginx", "HTTP"}},
		{ID: "disk-exhaustion", Title: "Disk Exhaustion", Difficulty: "Beginner", DurationMin: 18, Technologies: []string{"Linux", "Storage", "Logs"}},
		{ID: "postgres-pool-exhaustion", Title: "Checkout API Suddenly Became Slow", Difficulty: "Intermediate", DurationMin: 20, Technologies: []string{"PostgreSQL", "Nginx", "Redis"}},
	}}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		respond(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/scenarios", s.listScenarios)
	mux.HandleFunc("POST /api/sessions", s.createSession)
	mux.HandleFunc("GET /api/sessions/{id}", s.getSession)
	mux.HandleFunc("POST /api/sessions/{id}/reset", s.resetSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.destroySession)
	mux.HandleFunc("POST /api/sessions/{id}/submit", s.submitSession)
	mux.HandleFunc("GET /api/sessions/{id}/timeline", s.timeline)
	mux.HandleFunc("GET /ws/sessions/{id}/terminal", s.terminal)
	return requestID(mux)
}
func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	user, err := userID(r)
	if err != nil {
		fail(w, 401, "AUTH_REQUIRED", "Authentication required.")
		return
	}
	session, err := s.store.GetOwned(r.PathValue("id"), user)
	if err != nil {
		fail(w, 404, "SESSION_NOT_FOUND", "Session not found.")
		return
	}
	postgres, ok := s.store.(*store.PostgresStore)
	if !ok {
		respond(w, 200, []any{})
		return
	}
	events, err := postgres.Timeline(session.ID)
	if err != nil {
		fail(w, 500, "TIMELINE_UNAVAILABLE", "Timeline is unavailable.")
		return
	}
	audit, err := postgres.AuditTimeline(session.ID)
	if err != nil { fail(w, 500, "TIMELINE_UNAVAILABLE", "Timeline is unavailable."); return }
	attempts, err := postgres.Attempts(session.ID)
	if err != nil { fail(w, 500, "TIMELINE_UNAVAILABLE", "Timeline is unavailable."); return }
	respond(w, 200, map[string]any{"terminal": events, "lifecycle": audit, "attempts": attempts})
}
func (s *Server) submitSession(w http.ResponseWriter, r *http.Request) {
	user, err := userID(r)
	if err != nil {
		fail(w, 401, "AUTH_REQUIRED", "Authentication required.")
		return
	}
	session, err := s.store.GetOwned(r.PathValue("id"), user)
	if err != nil {
		fail(w, 404, "SESSION_NOT_FOUND", "Session not found.")
		return
	}
	if session.ScenarioID != "broken-nginx-upstream" || session.State != domain.StateRunning {
		fail(w, 409, "INVALID_STATE", "Session cannot be submitted now.")
		return
	}
	if err := validation.NginxHealthy(r.Context(), session.RuntimeID); err != nil {
		fail(w, 422, "VALIDATION_FAILED", "The incident is not repaired yet.")
		return
	}
	session, err = s.store.UpdateOwned(session.ID, user, func(item *domain.Session) error { return item.Transition(domain.StateCompleted) })
	if err != nil {
		fail(w, 409, "INVALID_STATE", "Session cannot be completed now.")
		return
	}
	if postgres, ok := s.store.(*store.PostgresStore); ok {
		if err := postgres.RecordSubmission(session.ID, 100); err != nil {
			s.log.Error("submission persistence failed", "session_id", session.ID, "error", err)
		}
		_ = postgres.RecordAudit(session.ID, "INCIDENT_COMPLETED")
	}
	respond(w, 200, map[string]any{"session": session, "score": 100})
}

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || strings.Contains(origin, r.Host)
	},
}

func (s *Server) terminal(w http.ResponseWriter, r *http.Request) {
	user, err := userID(r)
	if err != nil {
		fail(w, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication required.")
		return
	}
	session, err := s.store.GetOwned(r.PathValue("id"), user)
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session not found.")
		return
	}
	if session.State != domain.StateRunning || session.ScenarioID != "broken-nginx-upstream" {
		fail(w, http.StatusConflict, "TERMINAL_UNAVAILABLE", "Terminal is unavailable for this session.")
		return
	}
	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Terminal connection failed safely."))
		return
	}
	defer docker.Close()
	target := "kq-" + session.RuntimeID + "-nginx"
	execID, err := docker.ContainerExecCreate(r.Context(), target, container.ExecOptions{AttachStdin: true, AttachStdout: true, AttachStderr: true, Tty: true, Cmd: []string{"/bin/sh"}})
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Terminal environment is unavailable."))
		return
	}
	stream, err := docker.ContainerExecAttach(r.Context(), execID.ID, container.ExecAttachOptions{Tty: true})
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Terminal stream is unavailable."))
		return
	}
	defer stream.Close()
	var closeOnce sync.Once
	closeStream := func() { closeOnce.Do(func() { stream.Close() }) }
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 4096)
		for {
			count, readErr := stream.Reader.Read(buffer)
			if count > 0 && conn.WriteMessage(websocket.BinaryMessage, buffer[:count]) != nil {
				closeStream()
				return
			}
			if readErr != nil {
				return
			}
		}
	}()
	commandBuffer := ""
	for {
		_, input, readErr := conn.ReadMessage()
		if readErr != nil {
			closeStream()
			<-done
			return
		}
		for _, character := range string(input) {
			switch character {
			case '\r', '\n':
				if command := strings.TrimSpace(commandBuffer); command != "" {
					if postgres, ok := s.store.(*store.PostgresStore); ok {
						_ = postgres.RecordCommand(session.ID, command)
					}
				}
				commandBuffer = ""
			case '\b', 127:
				if length := len(commandBuffer); length > 0 {
					commandBuffer = commandBuffer[:length-1]
				}
			default:
				commandBuffer += string(character)
			}
		}
		if _, writeErr := stream.Conn.Write(input); writeErr != nil {
			closeStream()
			<-done
			return
		}
	}
}

func (s *Server) listScenarios(w http.ResponseWriter, _ *http.Request) {
	respond(w, http.StatusOK, s.scenarios)
}
func userID(r *http.Request) (string, error) {
	if id := strings.TrimSpace(r.Header.Get("X-Development-User")); id != "" {
		return id, nil
	}
	if id := strings.TrimSpace(r.URL.Query().Get("devUser")); id == "dev-user" {
		return id, nil
	}
	return "", errors.New("authentication required")
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	user, err := userID(r)
	if err != nil {
		fail(w, http.StatusUnauthorized, "AUTH_REQUIRED", "Use the development login header.")
		return
	}
	var input struct {
		ScenarioID string `json:"scenarioId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		fail(w, 400, "INVALID_REQUEST", "Invalid JSON request.")
		return
	}
	found := false
	for _, scenario := range s.scenarios {
		if scenario.ID == input.ScenarioID {
			found = true
		}
	}
	if !found {
		fail(w, 404, "SCENARIO_NOT_FOUND", "Scenario not found.")
		return
	}
	session := domain.Session{ID: newID(), UserID: user, ScenarioID: input.ScenarioID, State: domain.StateCreating, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(45 * time.Minute)}
	if err := s.store.Create(session); err != nil {
		fail(w, 409, "SESSION_CONFLICT", "Unable to create a session.")
		return
	}
	go s.provision(context.Background(), session.ID, user, session.ScenarioID)
	if postgres, ok := s.store.(*store.PostgresStore); ok {
		_ = postgres.RecordAttempt(session.ID)
		_ = postgres.RecordAudit(session.ID, "SESSION_CREATED")
	}
	respond(w, http.StatusAccepted, session)
}
func (s *Server) provision(ctx context.Context, id, user, scenarioID string) {
	_, err := s.store.UpdateOwned(id, user, func(session *domain.Session) error { return session.Transition(domain.StateProvisioning) })
	if err != nil {
		return
	}
	runtimeID, err := s.runtime.Provision(ctx, id, scenarioID)
	if err != nil {
		s.log.Error("runtime provisioning failed", "session_id", id, "error", err)
		_, _ = s.store.UpdateOwned(id, user, func(session *domain.Session) error { return session.Transition(domain.StateDestroyed) })
		return
	}
	_, err = s.store.UpdateOwned(id, user, func(session *domain.Session) error {
		if session.State != domain.StateProvisioning {
			return errors.New("provisioning cancelled")
		}
		session.RuntimeID = runtimeID
		if err := session.Transition(domain.StateReady); err != nil {
			return err
		}
		return session.Transition(domain.StateRunning)
	})
	if err != nil {
		if cleanupErr := s.runtime.Destroy(context.Background(), runtimeID); cleanupErr != nil {
			s.log.Error("cancelled runtime cleanup failed", "session_id", id, "runtime_id", runtimeID, "error", cleanupErr)
		}
	}
	if postgres, ok := s.store.(*store.PostgresStore); ok { _ = postgres.RecordAudit(id, "ENVIRONMENT_PROVISIONED") }
}
func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	user, err := userID(r)
	if err != nil {
		fail(w, 401, "AUTH_REQUIRED", "Authentication required.")
		return
	}
	session, err := s.store.GetOwned(r.PathValue("id"), user)
	if errors.Is(err, store.ErrNotFound) {
		fail(w, 404, "SESSION_NOT_FOUND", "Session not found.")
		return
	}
	respond(w, 200, session)
}
func (s *Server) resetSession(w http.ResponseWriter, r *http.Request) {
	user, err := userID(r)
	if err != nil {
		fail(w, 401, "AUTH_REQUIRED", "Authentication required.")
		return
	}
	session, err := s.store.UpdateOwned(r.PathValue("id"), user, func(session *domain.Session) error { return session.Transition(domain.StateResetting) })
	if err != nil {
		fail(w, 409, "INVALID_STATE", "Session cannot be reset now.")
		return
	}
	if err := s.runtime.Destroy(r.Context(), session.RuntimeID); err != nil {
		s.log.Error("reset cleanup failed", "session_id", session.ID, "error", err)
	}
	go s.provision(context.Background(), session.ID, user, session.ScenarioID)
	if postgres, ok := s.store.(*store.PostgresStore); ok {
		_ = postgres.RecordAttempt(session.ID)
		_ = postgres.RecordAudit(session.ID, "RESET_REQUESTED")
	}
	respond(w, 202, session)
}
func (s *Server) destroySession(w http.ResponseWriter, r *http.Request) {
	user, err := userID(r)
	if err != nil {
		fail(w, 401, "AUTH_REQUIRED", "Authentication required.")
		return
	}
	session, err := s.store.GetOwned(r.PathValue("id"), user)
	if errors.Is(err, store.ErrNotFound) {
		fail(w, 404, "SESSION_NOT_FOUND", "Session not found.")
		return
	}
	if session.State != domain.StateDestroyed {
		if err := s.runtime.Destroy(r.Context(), session.RuntimeID); err != nil {
			fail(w, 502, "RUNTIME_ERROR", "Environment cleanup failed safely. Try again.")
			return
		}
		session, err = s.store.UpdateOwned(session.ID, user, func(item *domain.Session) error {
			if item.State == domain.StateDestroyed {
				return nil
			}
			return item.Transition(domain.StateDestroyed)
		})
		if err != nil {
			fail(w, 409, "INVALID_STATE", "Session cannot be destroyed now.")
			return
		}
	}
	if postgres, ok := s.store.(*store.PostgresStore); ok { _ = postgres.RecordAudit(session.ID, "ENVIRONMENT_DESTROYED") }
	respond(w, 200, session)
}
func newID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Development-User")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("X-Request-ID", newID())
		next.ServeHTTP(w, r)
	})
}
