package src

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	mcp "github.com/Back-to-code/go-mcp"
	"github.com/invopop/jsonschema"
	"github.com/mxschmitt/playwright-go"
)

// AriaOption is either a boolean or a query string that narrows the dump.
type AriaOption struct {
	Enabled bool
	Query   string
}

func (a *AriaOption) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	switch {
	case bytes.Equal(data, []byte("null")), bytes.Equal(data, []byte("false")):
		*a = AriaOption{}
	case bytes.Equal(data, []byte("true")):
		*a = AriaOption{Enabled: true}
	default:
		var q string
		if err := json.Unmarshal(data, &q); err != nil {
			return errors.New("aria must be true, false or a query string")
		}
		q = strings.TrimSpace(q)
		*a = AriaOption{Enabled: q != "" && q != "false", Query: q}
		if q == "true" {
			a.Query = ""
		}
	}
	return nil
}

func (AriaOption) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{{Type: "boolean"}, {Type: "string"}},
	}
}

// Output is embedded in every tool that acts on a page.
type Output struct {
	Wait       *float64   `json:"wait,omitempty" jsonschema_description:"Seconds to wait after the action before the aria dump / screenshot is taken (default 0.5, max 30)."`
	Aria       AriaOption `json:"aria" jsonschema_description:"Return the aria dump of the page after the action. false: no dump, true: the full dump, a string: only the parts of the dump whose line contains this text (case insensitive) with their subtree and ancestors, or when it is an element id like \"e12\" only that element and its subtree."`
	Screenshot bool       `json:"screenshot" jsonschema_description:"Take a screenshot of the viewport after the action. The result contains a url to the png which is deleted after 5 minutes."`
}

type SessionArgs struct {
	SessionID string `json:"session_id" jsonschema_description:"The session id returned by start_session."`
}

type StartSessionArgs struct {
	URL            string   `json:"url" jsonschema_description:"The url to open in the session tab."`
	AllowedDomains []string `json:"allowed_domains,omitempty" jsonschema_description:"Domains the tab may navigate to, each domain also allows its subdomains. Defaults to the domain of url. When the tab navigates outside of these domains the session is closed."`
	Resolution     string   `json:"resolution,omitempty" jsonschema:"enum=desktop,enum=iphone,enum=ipad_landscape" jsonschema_description:"Viewport preset, defaults to desktop (1440x900). iphone is 390x844, ipad_landscape is 1180x820."`
	Output
}

type StateArgs struct {
	SessionArgs
	Output
}

type ClickByIDArgs struct {
	SessionArgs
	ID string `json:"id" jsonschema_description:"Element id from the aria dump, e.g. e12."`
	Output
}

type ClickByTextArgs struct {
	SessionArgs
	Text  string `json:"text" jsonschema_description:"Visible text, aria-label or title of the element to click."`
	Exact bool   `json:"exact,omitempty" jsonschema_description:"Only match the full text case sensitive, by default an exact match is preferred and a case insensitive substring match is the fallback."`
	Nth   int    `json:"nth,omitempty" jsonschema_description:"Zero based index of the match to use when multiple elements match (default 0)."`
	Output
}

type TypeByIDArgs struct {
	SessionArgs
	ID     string `json:"id" jsonschema_description:"Element id from the aria dump, e.g. e12. The element is clicked to focus it before typing."`
	Text   string `json:"text" jsonschema_description:"The text to type."`
	Clear  bool   `json:"clear,omitempty" jsonschema_description:"Select all and delete the existing value before typing."`
	Submit bool   `json:"submit,omitempty" jsonschema_description:"Press Enter after typing."`
	Output
}

type TypeByTextArgs struct {
	SessionArgs
	Target string `json:"target" jsonschema_description:"Label, placeholder or text of the input to type in. The element is clicked to focus it before typing."`
	Text   string `json:"text" jsonschema_description:"The text to type."`
	Clear  bool   `json:"clear,omitempty" jsonschema_description:"Select all and delete the existing value before typing."`
	Submit bool   `json:"submit,omitempty" jsonschema_description:"Press Enter after typing."`
	Exact  bool   `json:"exact,omitempty" jsonschema_description:"Only match the full target text case sensitive."`
	Nth    int    `json:"nth,omitempty" jsonschema_description:"Zero based index of the match to use when multiple elements match (default 0)."`
	Output
}

type PressKeyArgs struct {
	SessionArgs
	Keys []string `json:"keys" jsonschema:"minItems=1" jsonschema_description:"Keys or shortcuts pressed in order on the page, e.g. [\"Escape\"], [\"Control+k\"], [\"Tab\", \"Tab\", \"Enter\"]. Modifiers: Shift, Control, Alt, Meta, ControlOrMeta. Key names follow KeyboardEvent.key (ArrowDown, PageDown, Backspace, a, ...)."`
	Output
}

type ScrollArgs struct {
	SessionArgs
	X      float64 `json:"x" jsonschema_description:"X position in the viewport (css pixels) where the mouse wheel is used, this decides which scroll container scrolls."`
	Y      float64 `json:"y" jsonschema_description:"Y position in the viewport (css pixels) where the mouse wheel is used."`
	DeltaX float64 `json:"delta_x,omitempty" jsonschema_description:"Pixels to scroll horizontally, positive scrolls right."`
	DeltaY float64 `json:"delta_y,omitempty" jsonschema_description:"Pixels to scroll vertically, positive scrolls down."`
	Output
}

type SetResolutionArgs struct {
	SessionArgs
	Resolution string `json:"resolution" jsonschema:"enum=desktop,enum=iphone,enum=ipad_landscape" jsonschema_description:"desktop is 1440x900, iphone is 390x844, ipad_landscape is 1180x820. Only the viewport size changes, not the user agent."`
	Output
}

type NavigateArgs struct {
	SessionArgs
	URL string `json:"url" jsonschema_description:"Url to open, must be within the allowed domains of the session."`
	Output
}

type SelectOptionArgs struct {
	SessionArgs
	ID     string `json:"id" jsonschema_description:"Id of a native select (combobox/listbox) from the aria dump."`
	Option string `json:"option" jsonschema_description:"Label or value of the option to select."`
	Output
}

func (a SessionArgs) sessionID() string { return a.SessionID }

type callSessionKey struct{}

type callDetailKey struct{}

// setCallSession sets the session logged for the current tool call, for tools
// like start_session that only know the session after the call started.
func setCallSession(ctx context.Context, id string) {
	if p, ok := ctx.Value(callSessionKey{}).(*string); ok {
		*p = id
	}
}

// setCallDetail sets what the tool call did, it is added to the log line.
func setCallDetail(ctx context.Context, detail string) {
	if p, ok := ctx.Value(callDetailKey{}).(*string); ok {
		*p = detail
	}
}

// outputDetail describes what a page tool returned, for the log line.
func outputDetail(out Output) string {
	var parts []string
	if out.Aria.Query != "" {
		parts = append(parts, fmt.Sprintf("aria %q", out.Aria.Query))
	} else if out.Aria.Enabled {
		parts = append(parts, "aria")
	}
	if out.Screenshot {
		parts = append(parts, "screenshot")
	}
	if len(parts) == 0 {
		return ""
	}
	return " (+" + strings.Join(parts, ", ") + ")"
}

// addTool registers a tool and logs every call with its session, outcome and duration.
func addTool[T any](server *mcp.Server, tool mcp.Tool[T]) {
	handler := tool.Handler
	tool.Handler = func(args T, ctx context.Context) (any, error) {
		session := ServerLogID
		if a, ok := any(args).(interface{ sessionID() string }); ok {
			session = a.sessionID()
		}
		detail := ""
		ctx = context.WithValue(ctx, callSessionKey{}, &session)
		ctx = context.WithValue(ctx, callDetailKey{}, &detail)
		start := time.Now()
		res, err := handler(args, ctx)
		took := time.Since(start).Round(time.Millisecond)
		if err != nil {
			msg, _, _ := strings.Cut(err.Error(), "\n")
			Logf(session, "%s failed after %s: %s", tool.Name, took, msg)
		} else if detail != "" {
			Logf(session, "%s ok after %s: %s", tool.Name, took, detail)
		} else {
			Logf(session, "%s ok after %s", tool.Name, took)
		}
		return res, err
	}
	mcp.AddToolToServer(server, tool)
}

const outputHelp = " Every page tool waits `wait` seconds after the action and can return the aria dump and/or a screenshot url. " +
	"Aria dump format: one element per line, indentation is nesting, `[e12]` is the id to use with the *_by_id tools, " +
	"`heading[2]` is the heading level, quoted text is text content, `-> /path` is a link target."

func globalDomainsHelp(global []string) string {
	if len(global) == 0 {
		return ""
	}
	return " This server only allows sessions for these domains (and their subdomains): " + strings.Join(global, ", ") + "."
}

func RegisterTools(server *mcp.Server, m *Manager) {
	withSession := func(ctx context.Context, id string, out Output, action func(s *Session) (string, error)) (any, error) {
		s, err := m.Get(id)
		if err != nil {
			return nil, err
		}
		return s.run(out, func() (string, error) {
			msg, err := action(s)
			if err == nil {
				setCallDetail(ctx, msg+outputDetail(out))
			}
			return msg, err
		})
	}

	addTool(server, mcp.Tool[StartSessionArgs]{
		Name: "start_session",
		Description: "Start a browser session with a single headless firefox tab and open url in it. " +
			"The tab may only visit the allowed domains, navigating elsewhere closes the session. " +
			"Sessions are closed after 5 minutes without any tool call, use keep_alive to extend. " +
			"Dialogs (alert/confirm/prompt) are accepted automatically and reported as events. Always stop_session when done." +
			globalDomainsHelp(m.cfg.GlobalAllowedDomains) + outputHelp,
		Handler: func(args StartSessionArgs, ctx context.Context) (any, error) {
			u, err := url.Parse(args.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
				return nil, fmt.Errorf("url must be an absolute http(s) url")
			}
			domains := args.AllowedDomains
			if len(domains) == 0 {
				domains = []string{u.Hostname()}
			}
			allowed, err := normalizeDomains(domains)
			if err != nil {
				return nil, err
			}
			if err := checkGlobalDomains(allowed, m.cfg.GlobalAllowedDomains); err != nil {
				return nil, err
			}
			if !domainAllowed(args.URL, allowed) {
				return nil, fmt.Errorf("url %s is not within allowed_domains", args.URL)
			}
			s, err := m.Create(args.URL, allowed, args.Resolution)
			if err != nil {
				return nil, errors.New(cleanErr(err))
			}
			setCallSession(ctx, s.ID)
			setCallDetail(ctx, args.URL+outputDetail(args.Output))
			return s.run(args.Output, func() (string, error) {
				return fmt.Sprintf("session started, allowed domains: %s", strings.Join(allowed, ", ")), nil
			})
		},
	})

	// Not wrapped in addTool so polling the count does not flood the logs.
	mcp.AddToolToServer(server, mcp.Tool[struct{}]{
		Name:        "session_count",
		Description: "Returns the number of open sessions on this server.",
		Handler: func(_ struct{}, ctx context.Context) (any, error) {
			return m.Count(), nil
		},
	})

	addTool(server, mcp.Tool[SessionArgs]{
		Name:        "stop_session",
		Description: "Stop a browser session and close its tab.",
		Handler: func(args SessionArgs, ctx context.Context) (any, error) {
			s, err := m.Get(args.SessionID)
			if err != nil {
				return nil, err
			}
			s.close("stopped by stop_session")
			return "session " + s.ID + " stopped", nil
		},
	})

	addTool(server, mcp.Tool[SessionArgs]{
		Name:        "keep_alive",
		Description: "Reset the inactivity timer of a session so it is not closed automatically.",
		Handler: func(args SessionArgs, ctx context.Context) (any, error) {
			s, err := m.Get(args.SessionID)
			if err != nil {
				return nil, err
			}
			if err := s.closedErr(); err != nil {
				return nil, err
			}
			timeout := s.touch()
			return fmt.Sprintf("session %s will be closed after %s without activity", s.ID, timeout), nil
		},
	})

	addTool(server, mcp.Tool[StateArgs]{
		Name:        "get_page",
		Description: "Get the current state of the page (aria dump and/or screenshot) without doing anything." + outputHelp,
		Handler: func(args StateArgs, ctx context.Context) (any, error) {
			return withSession(ctx, args.SessionID, args.Output, func(s *Session) (string, error) {
				return "ok", nil
			})
		},
	})

	addTool(server, mcp.Tool[ClickByIDArgs]{
		Name:        "click_by_id",
		Description: "Click the center of an element from the aria dump using real (trusted) mouse events. Scrolls it into view first." + outputHelp,
		Handler: func(args ClickByIDArgs, ctx context.Context) (any, error) {
			return withSession(ctx, args.SessionID, args.Output, func(s *Session) (string, error) {
				el, err := s.element(args.ID)
				if err != nil {
					return "", err
				}
				defer el.Dispose()
				if err := el.Click(playwright.ElementHandleClickOptions{Timeout: playwright.Float(actionTimeoutMs)}); err != nil {
					return "", err
				}
				return "clicked " + args.ID, nil
			})
		},
	})

	addTool(server, mcp.Tool[ClickByTextArgs]{
		Name:        "click_by_text",
		Description: "Click a visible element found by its text, aria-label or title using real (trusted) mouse events." + outputHelp,
		Handler: func(args ClickByTextArgs, ctx context.Context) (any, error) {
			return withSession(ctx, args.SessionID, args.Output, func(s *Session) (string, error) {
				loc, desc, err := s.findByText(args.Text, args.Exact, args.Nth, false)
				if err != nil {
					return "", err
				}
				if err := loc.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(actionTimeoutMs)}); err != nil {
					return "", err
				}
				return "clicked " + desc, nil
			})
		},
	})

	addTool(server, mcp.Tool[TypeByIDArgs]{
		Name:        "type_by_id",
		Description: "Click an element from the aria dump to focus it and type text using real (trusted) keyboard events." + outputHelp,
		Handler: func(args TypeByIDArgs, ctx context.Context) (any, error) {
			return withSession(ctx, args.SessionID, args.Output, func(s *Session) (string, error) {
				el, err := s.element(args.ID)
				if err != nil {
					return "", err
				}
				defer el.Dispose()
				if err := el.Click(playwright.ElementHandleClickOptions{Timeout: playwright.Float(actionTimeoutMs)}); err != nil {
					return "", err
				}
				if err := s.typeText(args.Text, args.Clear, args.Submit); err != nil {
					return "", err
				}
				return fmt.Sprintf("typed %d characters into %s", len([]rune(args.Text)), args.ID), nil
			})
		},
	})

	addTool(server, mcp.Tool[TypeByTextArgs]{
		Name:        "type_by_text",
		Description: "Find an input by its label, placeholder or text, click it to focus it and type text using real (trusted) keyboard events." + outputHelp,
		Handler: func(args TypeByTextArgs, ctx context.Context) (any, error) {
			return withSession(ctx, args.SessionID, args.Output, func(s *Session) (string, error) {
				loc, desc, err := s.findByText(args.Target, args.Exact, args.Nth, true)
				if err != nil {
					return "", err
				}
				if err := loc.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(actionTimeoutMs)}); err != nil {
					return "", err
				}
				if err := s.typeText(args.Text, args.Clear, args.Submit); err != nil {
					return "", err
				}
				return fmt.Sprintf("typed %d characters into %s", len([]rune(args.Text)), desc), nil
			})
		},
	})

	addTool(server, mcp.Tool[PressKeyArgs]{
		Name:        "press_key",
		Description: "Press keys or keyboard shortcuts on the page (sent to whatever element has focus) using real (trusted) keyboard events." + outputHelp,
		Handler: func(args PressKeyArgs, ctx context.Context) (any, error) {
			return withSession(ctx, args.SessionID, args.Output, func(s *Session) (string, error) {
				if len(args.Keys) == 0 {
					return "", errors.New("keys is empty")
				}
				for _, key := range args.Keys {
					if err := s.page.Keyboard().Press(key); err != nil {
						return "", err
					}
				}
				return "pressed " + strings.Join(args.Keys, ", "), nil
			})
		},
	})

	addTool(server, mcp.Tool[ScrollArgs]{
		Name:        "scroll",
		Description: "Move the mouse to x,y in the viewport and use the mouse wheel to scroll by delta_x/delta_y pixels (trusted wheel events)." + outputHelp,
		Handler: func(args ScrollArgs, ctx context.Context) (any, error) {
			return withSession(ctx, args.SessionID, args.Output, func(s *Session) (string, error) {
				if err := s.scroll(args.X, args.Y, args.DeltaX, args.DeltaY); err != nil {
					return "", err
				}
				return fmt.Sprintf("scrolled %v,%v at %v,%v", args.DeltaX, args.DeltaY, args.X, args.Y), nil
			})
		},
	})

	addTool(server, mcp.Tool[SetResolutionArgs]{
		Name:        "set_resolution",
		Description: "Change the viewport size of the session tab to one of the presets." + outputHelp,
		Handler: func(args SetResolutionArgs, ctx context.Context) (any, error) {
			return withSession(ctx, args.SessionID, args.Output, func(s *Session) (string, error) {
				if err := s.setResolution(args.Resolution); err != nil {
					return "", err
				}
				return "resolution set to " + args.Resolution, nil
			})
		},
	})

	addTool(server, mcp.Tool[NavigateArgs]{
		Name:        "navigate",
		Description: "Open a url in the session tab, the url must be within the allowed domains of the session." + outputHelp,
		Handler: func(args NavigateArgs, ctx context.Context) (any, error) {
			return withSession(ctx, args.SessionID, args.Output, func(s *Session) (string, error) {
				if err := s.navigate(args.URL); err != nil {
					return "", err
				}
				return "navigated to " + args.URL, nil
			})
		},
	})

	addTool(server, mcp.Tool[SelectOptionArgs]{
		Name:        "select_option",
		Description: "Select an option of a native <select> element by its label or value." + outputHelp,
		Handler: func(args SelectOptionArgs, ctx context.Context) (any, error) {
			return withSession(ctx, args.SessionID, args.Output, func(s *Session) (string, error) {
				el, err := s.element(args.ID)
				if err != nil {
					return "", err
				}
				defer el.Dispose()
				opts := playwright.ElementHandleSelectOptionOptions{Timeout: playwright.Float(actionTimeoutMs)}
				selected, err := el.SelectOption(playwright.SelectOptionValues{Labels: &[]string{args.Option}}, opts)
				if err != nil || len(selected) == 0 {
					selected, err = el.SelectOption(playwright.SelectOptionValues{Values: &[]string{args.Option}}, opts)
				}
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("selected %q in %s", args.Option, args.ID), nil
			})
		},
	})
}
