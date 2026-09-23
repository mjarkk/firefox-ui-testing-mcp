package src

import (
	"fmt"
	"net/url"
	"strings"
)

// normalizeDomains turns user input like "https://www.Example.com:3000/x" or
// "*.example.com" into bare lowercase host names.
func normalizeDomains(domains []string) ([]string, error) {
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimPrefix(d, "*.")
		if strings.Contains(d, "://") {
			u, err := url.Parse(d)
			if err != nil {
				return nil, fmt.Errorf("invalid domain %q: %w", d, err)
			}
			d = u.Hostname()
		} else {
			d, _, _ = strings.Cut(d, "/")
			if host, _, ok := strings.Cut(d, ":"); ok && !strings.HasPrefix(d, "[") {
				d = host
			}
		}
		d = strings.Trim(d, ".[]")
		if d == "" {
			return nil, fmt.Errorf("empty domain in allowed_domains")
		}
		out = append(out, d)
	}
	return out, nil
}

// domainAllowed reports whether the url may be shown in the session tab.
// A domain also allows all of its subdomains.
func domainAllowed(rawURL string, allowed []string) bool {
	switch {
	case rawURL == "", rawURL == "about:blank", rawURL == "about:srcdoc":
		return true
	case strings.HasPrefix(rawURL, "blob:"):
		rawURL = strings.TrimPrefix(rawURL, "blob:")
	}

	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return hostWithin(strings.TrimSuffix(strings.ToLower(u.Hostname()), "."), allowed)
}

func hostWithin(host string, domains []string) bool {
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// checkGlobalDomains makes sure every session domain lies within the global
// domains, so enforcing the session domains also enforces the global ones.
// An empty global list allows everything.
func checkGlobalDomains(session, global []string) error {
	if len(global) == 0 {
		return nil
	}
	for _, d := range session {
		if !hostWithin(d, global) {
			return fmt.Errorf("domain %s is not allowed by this server, allowed domains are: %s", d, strings.Join(global, ", "))
		}
	}
	return nil
}
