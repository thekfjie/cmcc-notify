package main

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gotify/plugin-api"
)

type persistedState struct {
	Seen map[string]time.Time `json:"seen"`
}

type stateStore struct {
	mu      sync.Mutex
	handler plugin.StorageHandler
	seen    map[string]time.Time
}

func (s *stateStore) setHandler(handler plugin.StorageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
	s.seen = make(map[string]time.Time)
	if handler == nil {
		return
	}
	b, err := handler.Load()
	if err == nil && len(b) > 0 {
		var state persistedState
		if json.Unmarshal(b, &state) == nil && state.Seen != nil {
			s.seen = state.Seen
		}
	}
}

func (s *stateStore) seenRecently(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen == nil {
		s.seen = make(map[string]time.Time)
	}
	if at, ok := s.seen[key]; ok && time.Since(at) < 24*time.Hour {
		return true
	}
	return false
}

func (s *stateStore) markSeen(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen == nil {
		s.seen = make(map[string]time.Time)
	}
	s.seen[key] = time.Now()
	if len(s.seen) > 5000 {
		cutoff := time.Now().Add(-24 * time.Hour)
		for k, at := range s.seen {
			if at.Before(cutoff) {
				delete(s.seen, k)
			}
		}
	}
	if s.handler != nil {
		if b, err := json.Marshal(persistedState{Seen: s.seen}); err == nil {
			_ = s.handler.Save(b)
		}
	}
}
