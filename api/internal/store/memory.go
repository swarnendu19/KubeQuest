package store

import (
	"errors"
	"sync"
	"time"

	"github.com/swarnendu-maity/kernelquest/api/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	Create(domain.Session) error
	GetOwned(string, string) (domain.Session, error)
	UpdateOwned(string, string, func(*domain.Session) error) (domain.Session, error)
	ExpireBefore(time.Time) []domain.Session
}

type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]domain.Session
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{sessions: make(map[string]domain.Session)} }

func (s *MemoryStore) Create(session domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[session.ID]; exists {
		return errors.New("session already exists")
	}
	s.sessions[session.ID] = session
	return nil
}

func (s *MemoryStore) GetOwned(id, userID string) (domain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	if !ok || session.UserID != userID {
		return domain.Session{}, ErrNotFound
	}
	return session, nil
}

func (s *MemoryStore) UpdateOwned(id, userID string, update func(*domain.Session) error) (domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok || session.UserID != userID {
		return domain.Session{}, ErrNotFound
	}
	if err := update(&session); err != nil {
		return domain.Session{}, err
	}
	s.sessions[id] = session
	return session, nil
}

func (s *MemoryStore) ExpireBefore(now time.Time) []domain.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	expired := make([]domain.Session, 0)
	for id, session := range s.sessions {
		if session.State == domain.StateRunning && session.ExpiresAt.Before(now) {
			_ = session.Transition(domain.StateExpired)
			s.sessions[id] = session
			expired = append(expired, session)
		}
	}
	return expired
}
