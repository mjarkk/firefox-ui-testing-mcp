package src

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type screenshot struct {
	data    []byte
	expires time.Time
}

// ScreenshotStore keeps screenshots in memory until their TTL passes.
type ScreenshotStore struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]screenshot
}

func NewScreenshotStore(ttl time.Duration) *ScreenshotStore {
	s := &ScreenshotStore{ttl: ttl, items: map[string]screenshot{}}
	go s.cleanupLoop()
	return s
}

func (s *ScreenshotStore) Add(data []byte) string {
	id := randomID(16)
	s.mu.Lock()
	s.items[id] = screenshot{data: data, expires: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	return id
}

func (s *ScreenshotStore) Get(id string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok || time.Now().After(item.expires) {
		return nil, false
	}
	return item.data, true
}

func (s *ScreenshotStore) cleanupLoop() {
	for range time.Tick(30 * time.Second) {
		now := time.Now()
		s.mu.Lock()
		for id, item := range s.items {
			if now.After(item.expires) {
				delete(s.items, id)
			}
		}
		s.mu.Unlock()
	}
}

func randomID(bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
