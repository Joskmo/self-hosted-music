package session

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

type Store struct {
	mu       sync.Mutex
	sessions map[string]data
}

type data struct {
	username string
	expires  time.Time
}

func New() *Store {
	return &Store{sessions: make(map[string]data)}
}

func (s *Store) Create(username string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	s.sessions[token] = data{username: username, expires: time.Now().Add(24 * time.Hour)}
	return token
}

func (s *Store) Valid(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.sessions[token]
	if !ok || time.Now().After(d.expires) {
		delete(s.sessions, token)
		return "", false
	}
	return d.username, true
}

func (s *Store) ValidFromCookie(r *http.Request) (string, bool) {
	cookie, err := r.Cookie("session")
	if err != nil {
		return "", false
	}
	return s.Valid(cookie.Value)
}

func (s *Store) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *Store) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for t, exp := range s.sessions {
		if now.After(exp.expires) {
			delete(s.sessions, t)
		}
	}
}
