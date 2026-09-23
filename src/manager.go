package src

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
)

type closedSession struct {
	reason string
	at     time.Time
}

type Manager struct {
	cfg   Config
	shots *ScreenshotStore
	pw    *playwright.Playwright

	mu           sync.Mutex
	browser      playwright.Browser
	launchedAt   time.Time
	shuttingDown bool
	sessions     map[string]*Session
	closed       map[string]closedSession
}

// minRelaunchInterval prevents a crash loop when firefox dies right after
// starting; it will then only be started again by the next start_session.
const minRelaunchInterval = 10 * time.Second

func NewManager(cfg Config, shots *ScreenshotStore) (*Manager, error) {
	pw, err := playwright.Run(&playwright.RunOptions{Browsers: []string{"firefox"}})
	if err != nil {
		return nil, fmt.Errorf("starting playwright: %w", err)
	}
	m := &Manager{
		cfg:      cfg,
		shots:    shots,
		pw:       pw,
		sessions: map[string]*Session{},
		closed:   map[string]closedSession{},
	}
	m.mu.Lock()
	_, err = m.browserLocked()
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	go m.reapLoop()
	return m, nil
}

// browserLocked returns the shared firefox instance, relaunching it when it
// crashed. Caller must hold m.mu.
func (m *Manager) browserLocked() (playwright.Browser, error) {
	if m.browser != nil && m.browser.IsConnected() {
		return m.browser, nil
	}
	browser, err := m.pw.Firefox.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(!m.cfg.Headful),
		FirefoxUserPrefs: map[string]any{
			// Wheel scrolling should land immediately so the next dump reflects it.
			"general.smoothScroll": false,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("launching firefox: %w", err)
	}
	Logf(ServerLogID, "firefox %s started", browser.Version())
	browser.OnDisconnected(func(b playwright.Browser) {
		go m.browserClosed(b)
	})
	m.browser = browser
	m.launchedAt = time.Now()
	return browser, nil
}

func (m *Manager) browserClosed(b playwright.Browser) {
	m.mu.Lock()
	if m.shuttingDown || m.browser != b {
		m.mu.Unlock()
		return
	}
	m.browser = nil
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	sinceLaunch := time.Since(m.launchedAt)
	m.mu.Unlock()

	Logf(ServerLogID, "firefox closed unexpectedly after running for %s", sinceLaunch.Round(time.Second))
	for _, s := range sessions {
		s.close("firefox crashed, start a new session")
	}

	if sinceLaunch < minRelaunchInterval {
		Logf(ServerLogID, "not relaunching firefox, it was started less than %s ago", minRelaunchInterval)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shuttingDown || m.browser != nil {
		return
	}
	if _, err := m.browserLocked(); err != nil {
		Logf(ServerLogID, "relaunching firefox: %v", err)
	}
}

func (m *Manager) Create(startURL string, allowed []string, preset string) (*Session, error) {
	m.mu.Lock()
	if len(m.sessions) >= m.cfg.MaxSessions {
		m.mu.Unlock()
		return nil, fmt.Errorf("maximum of %d concurrent sessions reached, stop a session first", m.cfg.MaxSessions)
	}
	browser, err := m.browserLocked()
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	now := time.Now()
	s := &Session{ID: randomID(6), m: m, allowed: allowed, createdAt: now, lastActive: now, nextID: 1}
	m.sessions[s.ID] = s
	m.mu.Unlock()

	if err := s.open(browser, startURL, preset); err != nil {
		s.close("failed to start: " + cleanErr(err))
		return nil, err
	}
	Logf(s.ID, "session started on %s, allowed domains: %s, resolution: %s (open sessions: %d)",
		startURL, strings.Join(allowed, ", "), s.preset, m.Count())
	return s, nil
}

func (m *Manager) Get(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	if c, ok := m.closed[id]; ok {
		return nil, fmt.Errorf("session %s was closed: %s", id, c.reason)
	}
	return nil, errors.New("unknown session " + id)
}

func (m *Manager) forget(s *Session, reason string) {
	m.mu.Lock()
	delete(m.sessions, s.ID)
	m.closed[s.ID] = closedSession{reason: reason, at: time.Now()}
	open := len(m.sessions)
	m.mu.Unlock()
	Logf(s.ID, "session closed after %s: %s (open sessions: %d)", time.Since(s.createdAt).Round(time.Second), reason, open)
}

func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func (m *Manager) reapLoop() {
	for range time.Tick(10 * time.Second) {
		m.mu.Lock()
		var idle []*Session
		for _, s := range m.sessions {
			if s.idleFor() > IdleTimeout {
				idle = append(idle, s)
			}
		}
		for id, c := range m.closed {
			if time.Since(c.at) > time.Hour {
				delete(m.closed, id)
			}
		}
		m.mu.Unlock()

		for _, s := range idle {
			s.close(fmt.Sprintf("no activity for %s", IdleTimeout))
		}
	}
}

func (m *Manager) Shutdown() {
	m.mu.Lock()
	m.shuttingDown = true
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	browser := m.browser
	m.mu.Unlock()
	for _, s := range sessions {
		s.close("server shutting down")
	}
	if browser != nil {
		_ = browser.Close()
	}
	_ = m.pw.Stop()
}
