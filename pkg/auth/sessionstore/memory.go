package sessionstore

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Memory struct {
	mu sync.Mutex
	m  map[string]Session
}

func NewMemory() *Memory { return &Memory{m: map[string]Session{}} }
func (s *Memory) Get(ctx context.Context, id string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[id]
	if !ok || time.Now().After(v.ExpiresAt) {
		return Session{}, errors.New("session not found")
	}
	return v, nil
}
func (s *Memory) Set(ctx context.Context, sess Session) error {
	s.mu.Lock()
	if s.m == nil {
		s.m = map[string]Session{}
	}
	s.m[sess.ID] = sess
	s.mu.Unlock()
	return nil
}
func (s *Memory) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	delete(s.m, id)
	s.mu.Unlock()
	return nil
}
func (s *Memory) Touch(ctx context.Context, id string, exp time.Time) error {
	s.mu.Lock()
	if v, ok := s.m[id]; ok {
		v.ExpiresAt = exp
		s.m[id] = v
	}
	s.mu.Unlock()
	return nil
}
