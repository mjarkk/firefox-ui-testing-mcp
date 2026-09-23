package src

import "testing"

func TestDomainAllowed(t *testing.T) {
	allowed, err := normalizeDomains([]string{"Example.com", "https://app.test:3000/login", "*.other.io", "localhost:8080"})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"https://example.com/x":         true,
		"https://www.example.com":       true,
		"http://app.test:3000/":         true,
		"https://sub.other.io":          true,
		"http://localhost:5173/":        true,
		"about:blank":                   true,
		"blob:https://example.com/uuid": true,
		"https://notexample.com":        false,
		"https://example.com.evil.io":   false,
		"data:text/html,hi":             false,
		"file:///etc/passwd":            false,
		"https://evil.io/?example.com":  false,
	}
	for url, want := range cases {
		if got := domainAllowed(url, allowed); got != want {
			t.Errorf("domainAllowed(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestCheckGlobalDomains(t *testing.T) {
	global := []string{"google.com", "duckduckgo.com"}
	cases := map[string]bool{
		"google.com":      true,
		"maps.google.com": true,
		"duckduckgo.com":  true,
		"example.com":     false,
		"com":             false,
		"notgoogle.com":   false,
	}
	for domain, want := range cases {
		if got := checkGlobalDomains([]string{domain}, global) == nil; got != want {
			t.Errorf("checkGlobalDomains(%q) allowed = %v, want %v", domain, got, want)
		}
	}
	if checkGlobalDomains([]string{"example.com"}, nil) != nil {
		t.Error("an empty global list should allow everything")
	}
}
