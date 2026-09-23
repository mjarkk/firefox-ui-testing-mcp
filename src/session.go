package src

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
)

type viewportPreset struct {
	Width, Height int
}

var viewportPresets = map[string]viewportPreset{
	"desktop":        {1440, 900},
	"iphone":         {390, 844},
	"ipad_landscape": {1180, 820},
}

const (
	defaultWait     = 0.5
	maxWait         = 30.0
	actionTimeoutMs = 5000
	maxEvents       = 20
)

type Session struct {
	ID      string
	m       *Manager
	allowed []string

	// opMu serializes all tool calls for this session.
	opMu sync.Mutex
	ctx  playwright.BrowserContext
	page playwright.Page

	mu          sync.Mutex
	preset      string
	createdAt   time.Time
	lastActive  time.Time
	busy        bool
	closed      bool
	closeReason string
	events      []string
	nextID      int
}

func (s *Session) open(browser playwright.Browser, startURL, preset string) error {
	if preset == "" {
		preset = "desktop"
	}
	vp, ok := viewportPresets[preset]
	if !ok {
		return fmt.Errorf("unknown resolution %q", preset)
	}
	s.preset = preset

	ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport:          &playwright.Size{Width: vp.Width, Height: vp.Height},
		AcceptDownloads:   playwright.Bool(false),
		IgnoreHttpsErrors: playwright.Bool(s.m.cfg.IgnoreHTTPSErrors),
	})
	if err != nil {
		return fmt.Errorf("creating browser context: %w", err)
	}
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()

	page, err := ctx.NewPage()
	if err != nil {
		return fmt.Errorf("opening tab: %w", err)
	}
	s.page = page
	s.watch()

	return s.navigate(startURL)
}

// watch registers the page listeners. Playwright invokes them on its dispatch
// goroutine, so anything that talks back to the browser runs in a new goroutine.
func (s *Session) watch() {
	page := s.page

	s.ctx.OnPage(func(p playwright.Page) {
		if p == page {
			return
		}
		var once sync.Once
		closeTab := func(url string) {
			once.Do(func() {
				s.event("closed a new tab/popup for " + url + " (a session has a single tab, use navigate if you need that page)")
				go p.Close()
			})
		}
		p.OnRequest(func(r playwright.Request) {
			if r.IsNavigationRequest() {
				closeTab(r.URL())
			}
		})
		go func() {
			time.Sleep(2 * time.Second)
			closeTab(p.URL())
		}()
	})

	checkURL := func(url string) {
		if !domainAllowed(url, s.allowed) {
			go s.close("the tab navigated to " + url + " which is outside the allowed domains (" + strings.Join(s.allowed, ", ") + ")")
		}
	}
	page.OnRequest(func(r playwright.Request) {
		if r.IsNavigationRequest() && r.Frame() == page.MainFrame() {
			checkURL(r.URL())
		}
	})
	page.OnFrameNavigated(func(f playwright.Frame) {
		if f == page.MainFrame() {
			Logf(s.ID, "tab navigated to %s", f.URL())
			checkURL(f.URL())
		}
	})
	page.OnClose(func(playwright.Page) {
		go s.close("the tab was closed by the page")
	})
	page.OnDialog(func(d playwright.Dialog) {
		s.event(fmt.Sprintf("%s dialog %q was accepted", d.Type(), d.Message()))
		go d.Accept()
	})
	page.OnPageError(func(err error) {
		s.event("page error: " + err.Error())
	})
	page.OnConsole(func(msg playwright.ConsoleMessage) {
		if msg.Type() == "error" {
			s.quietEvent("console error: " + msg.Text())
		}
	})
}

// event records something for the next tool result and logs it.
func (s *Session) event(msg string) {
	Logf(s.ID, "%s", msg)
	s.quietEvent(msg)
}

// quietEvent records something for the next tool result without logging it.
func (s *Session) quietEvent(msg string) {
	if len(msg) > 300 {
		msg = msg[:299] + "…"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.events) >= maxEvents {
		s.events = s.events[1:]
	}
	s.events = append(s.events, msg)
}

func (s *Session) drainEvents() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev := s.events
	s.events = nil
	return ev
}

func (s *Session) close(reason string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.closeReason = reason
	ctx := s.ctx
	s.mu.Unlock()

	s.m.forget(s, reason)
	if ctx != nil {
		_ = ctx.Close()
	}
}

func (s *Session) closedErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("session %s was closed: %s", s.ID, s.closeReason)
	}
	return nil
}

func (s *Session) touch() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActive = time.Now()
	return IdleTimeout
}

func (s *Session) idleFor() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return 0
	}
	return time.Since(s.lastActive)
}

func (s *Session) setBusy(busy bool) {
	s.mu.Lock()
	s.busy = busy
	s.lastActive = time.Now()
	s.mu.Unlock()
}

// run executes an action while holding the session lock and appends the
// requested page state to the result.
func (s *Session) run(out Output, action func() (string, error)) (string, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := s.closedErr(); err != nil {
		return "", err
	}
	s.setBusy(true)
	defer s.setBusy(false)

	msg, actionErr := action()
	if err := s.closedErr(); err != nil {
		return "", err
	}
	if actionErr != nil {
		if !out.Aria.Enabled && !out.Screenshot {
			return "", errors.New(cleanErr(actionErr))
		}
		report, err := s.report(out, "failed")
		if err != nil {
			return "", actionErr
		}
		return "", fmt.Errorf("%s\n\n%s", cleanErr(actionErr), report)
	}
	return s.report(out, msg)
}

func (s *Session) report(out Output, msg string) (string, error) {
	wait := defaultWait
	if out.Wait != nil {
		wait = min(max(*out.Wait, 0), maxWait)
	}
	time.Sleep(time.Duration(wait * float64(time.Second)))
	if err := s.closedErr(); err != nil {
		return "", err
	}
	_ = s.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateDomcontentloaded,
		Timeout: playwright.Float(10000),
	})

	var b strings.Builder
	fmt.Fprintf(&b, "session: %s\nresult: %s\n", s.ID, msg)

	var snap *ariaSnapshot
	if out.Aria.Enabled {
		var err error
		if snap, err = s.dump(); err != nil {
			if cerr := s.closedErr(); cerr != nil {
				return "", cerr
			}
			return "", fmt.Errorf("creating aria dump: %s", cleanErr(err))
		}
	}

	if ev := s.drainEvents(); len(ev) > 0 {
		b.WriteString("events since last call:\n")
		for _, e := range ev {
			b.WriteString("  - " + e + "\n")
		}
	}

	if snap != nil {
		fmt.Fprintf(&b, "url: %s\ntitle: %s\n", snap.URL, snap.Title)
	} else {
		title, _ := s.page.Title()
		fmt.Fprintf(&b, "url: %s\ntitle: %s\n", s.page.URL(), title)
	}

	if out.Screenshot {
		data, err := s.page.Screenshot(playwright.PageScreenshotOptions{
			Type:    playwright.ScreenshotTypePng,
			Timeout: playwright.Float(15000),
		})
		if err != nil {
			if cerr := s.closedErr(); cerr != nil {
				return "", cerr
			}
			return "", fmt.Errorf("taking screenshot: %s", cleanErr(err))
		}
		id := s.m.shots.Add(data)
		fmt.Fprintf(&b, "screenshot: %s/screenshots/%s.png (deleted after %s)\n", s.m.cfg.PublicURL, id, ScreenshotTTL)
	}

	if snap != nil {
		vp := viewportPresets[s.preset]
		fmt.Fprintf(&b, "viewport: %s %dx%d, scrolled to %d,%d of %dx%d\n",
			s.preset, vp.Width, vp.Height, snap.ScrollX, snap.ScrollY, snap.ScrollW, snap.ScrollH)
		b.WriteString("aria:\n")
		b.WriteString(renderAria(snap, out.Aria.Query, MaxAriaChars))
	}
	return b.String(), nil
}

func (s *Session) dump() (*ariaSnapshot, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			// Usually the page was navigating and the execution context got destroyed.
			time.Sleep(300 * time.Millisecond)
			_ = s.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
				State:   playwright.LoadStateDomcontentloaded,
				Timeout: playwright.Float(5000),
			})
		}
		raw, err := s.page.Evaluate(ariaJS, s.nextID)
		if err != nil {
			lastErr = err
			if s.closedErr() != nil {
				break
			}
			continue
		}
		snap, err := parseAriaSnapshot(raw)
		if err != nil {
			return nil, err
		}
		s.nextID = max(s.nextID, snap.Next)
		return snap, nil
	}
	return nil, lastErr
}

func (s *Session) navigate(url string) error {
	if !domainAllowed(url, s.allowed) {
		return fmt.Errorf("%s is not within the allowed domains (%s)", url, strings.Join(s.allowed, ", "))
	}
	_, err := s.page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	})
	return err
}

func (s *Session) element(id string) (playwright.ElementHandle, error) {
	id = strings.Trim(strings.TrimSpace(id), "[]")
	if id != "" && id[0] >= '0' && id[0] <= '9' {
		id = "e" + id
	}
	h, err := s.page.EvaluateHandle(lookupJS, id)
	if err != nil {
		return nil, err
	}
	el := h.AsElement()
	if el == nil {
		_ = h.Dispose()
		return nil, fmt.Errorf("no element with id %s on the current page, the page may have changed; request a fresh aria dump", id)
	}
	return el, nil
}

type textCandidate struct {
	desc string
	loc  playwright.Locator
}

// findByText looks for a visible element trying the strategies in order and
// returns the nth match of the first strategy that has one.
func (s *Session) findByText(text string, exact bool, nth int, forTyping bool) (playwright.Locator, string, error) {
	p := s.page
	byText := func(e bool) playwright.Locator {
		return p.GetByText(text, playwright.PageGetByTextOptions{Exact: playwright.Bool(e)})
	}
	byLabel := func(e bool) playwright.Locator {
		return p.GetByLabel(text, playwright.PageGetByLabelOptions{Exact: playwright.Bool(e)})
	}
	byPlaceholder := func(e bool) playwright.Locator {
		return p.GetByPlaceholder(text, playwright.PageGetByPlaceholderOptions{Exact: playwright.Bool(e)})
	}
	byTitle := func(e bool) playwright.Locator {
		return p.GetByTitle(text, playwright.PageGetByTitleOptions{Exact: playwright.Bool(e)})
	}

	var candidates []textCandidate
	if forTyping {
		candidates = []textCandidate{{"label", byLabel(true)}, {"placeholder", byPlaceholder(true)}}
		if !exact {
			candidates = append(candidates, textCandidate{"label", byLabel(false)}, textCandidate{"placeholder", byPlaceholder(false)})
		}
		candidates = append(candidates, textCandidate{"text", byText(true)})
		if !exact {
			candidates = append(candidates, textCandidate{"text", byText(false)})
		}
	} else {
		candidates = []textCandidate{{"text", byText(true)}, {"label", byLabel(true)}, {"title", byTitle(true)}}
		if !exact {
			candidates = append(candidates, textCandidate{"text", byText(false)}, textCandidate{"label", byLabel(false)}, textCandidate{"title", byTitle(false)})
		}
	}

	for _, c := range candidates {
		loc := c.loc.Filter(playwright.LocatorFilterOptions{Visible: playwright.Bool(true)})
		count, err := loc.Count()
		if err != nil {
			return nil, "", err
		}
		if count == 0 {
			continue
		}
		if nth >= count {
			return nil, "", fmt.Errorf("only %d visible elements match %s %q, nth %d is out of range", count, c.desc, text, nth)
		}
		desc := fmt.Sprintf("element with %s %q", c.desc, text)
		if count > 1 {
			desc += fmt.Sprintf(" (match %d of %d, use nth to pick another)", nth, count)
		}
		return loc.Nth(nth), desc, nil
	}
	return nil, "", fmt.Errorf("no visible element found for %q", text)
}

func (s *Session) typeText(text string, clear, submit bool) error {
	kb := s.page.Keyboard()
	if clear {
		if err := kb.Press("ControlOrMeta+a"); err != nil {
			return err
		}
		if err := kb.Press("Delete"); err != nil {
			return err
		}
	}
	if text != "" {
		if err := kb.Type(text); err != nil {
			return err
		}
	}
	if submit {
		return kb.Press("Enter")
	}
	return nil
}

func (s *Session) setResolution(preset string) error {
	vp, ok := viewportPresets[preset]
	if !ok {
		return fmt.Errorf("unknown resolution %q", preset)
	}
	if err := s.page.SetViewportSize(vp.Width, vp.Height); err != nil {
		return err
	}
	s.preset = preset
	return nil
}

func (s *Session) scroll(x, y, dx, dy float64) error {
	vp := viewportPresets[s.preset]
	if x < 0 || y < 0 || x >= float64(vp.Width) || y >= float64(vp.Height) {
		return fmt.Errorf("position %v,%v is outside of the %dx%d viewport", x, y, vp.Width, vp.Height)
	}
	mouse := s.page.Mouse()
	if err := mouse.Move(x, y); err != nil {
		return err
	}

	// Firefox clamps a single wheel event to roughly one page, so big deltas
	// are sent as multiple smaller wheel events.
	step := float64(min(vp.Width, vp.Height)) / 2
	steps := int(math.Ceil(max(math.Abs(dx), math.Abs(dy)) / step))
	for i := 0; i < steps; i++ {
		if err := mouse.Wheel(dx/float64(steps), dy/float64(steps)); err != nil {
			return err
		}
	}
	return nil
}

// cleanErr strips the long call logs playwright attaches to its errors.
func cleanErr(err error) string {
	var pwErr *playwright.Error
	msg := err.Error()
	if errors.As(err, &pwErr) {
		msg = pwErr.Message
	}
	lines := strings.Split(strings.TrimSpace(msg), "\n")
	if len(lines) > 8 {
		lines = append(lines[:8], "  …")
	}
	return strings.Join(lines, "\n")
}
