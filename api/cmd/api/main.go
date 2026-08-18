package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/swarnendu-maity/kernelquest/api/internal/httpapi"
	"github.com/swarnendu-maity/kernelquest/api/internal/runtime"
	"github.com/swarnendu-maity/kernelquest/api/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	var repository store.Repository = store.NewMemoryStore()
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		postgres, err := store.NewPostgres(context.Background(), databaseURL)
		if err != nil {
			logger.Error("database connection failed", "error", err)
			os.Exit(1)
		}
		defer postgres.Close()
		if err := postgres.Migrate(context.Background()); err != nil {
			logger.Error("database migration failed", "error", err)
			os.Exit(1)
		}
		repository = postgres
		logger.Info("PostgreSQL persistence enabled")
	}
	runtimeManager := runtime.DockerManager{}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			for _, session := range repository.ExpireBefore(time.Now().UTC()) {
				if postgres, ok := repository.(*store.PostgresStore); ok { _ = postgres.RecordAudit(session.ID, "INCIDENT_EXPIRED") }
				if err := runtimeManager.Destroy(context.Background(), session.RuntimeID); err != nil {
					logger.Error("expired runtime cleanup failed", "session_id", session.ID, "error", err)
				} else {
					if postgres, ok := repository.(*store.PostgresStore); ok { _ = postgres.RecordAudit(session.ID, "ENVIRONMENT_DESTROYED") }
					logger.Info("expired environment cleaned", "session_id", session.ID)
				}
			}
		}
	}()
	server := httpapi.New(repository, runtimeManager, logger)
	logger.Info("kernelquest control plane listening", "address", ":8080")
	if err := http.ListenAndServe(":8080", server.Handler()); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
