package src

import (
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/pflag"
)

const (
	IdleTimeout   = 5 * time.Minute
	ScreenshotTTL = 5 * time.Minute
	MaxAriaChars  = 40000
)

type Config struct {
	Port              string
	PublicURL         string
	MaxSessions       int
	IgnoreHTTPSErrors bool
	Headful           bool
	// GlobalAllowedDomains limits which domains sessions may allow, empty means no limit.
	GlobalAllowedDomains []string
}

// LoadConfig reads every option from its command line flag, then its env
// variable and otherwise uses the default.
func LoadConfig() Config {
	fs := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	port := fs.String("port", "8080", "port to listen on (env PORT)")
	publicURL := fs.String("public-url", "", "base url used in screenshot links, defaults to http://localhost:<port> (env PUBLIC_URL)")
	maxSessions := fs.Int("max-sessions", 50, "maximum number of concurrent sessions (env MAX_SESSIONS)")
	ignoreHTTPSErrors := fs.Bool("ignore-https-errors", false, "ignore invalid https certificates (env IGNORE_HTTPS_ERRORS)")
	headful := fs.Bool("headful", false, "show the firefox window instead of running headless, needs a display (env HEADFUL)")
	allowedDomains := fs.StringSlice("allowed-domains", nil, "comma separated domains (subdomains included) sessions may use, empty means no limit (env ALLOWED_DOMAINS)")
	_ = fs.Parse(os.Args[1:])

	cfg := Config{
		Port:              pick(fs, "port", "PORT", *port, parseString),
		PublicURL:         pick(fs, "public-url", "PUBLIC_URL", *publicURL, parseString),
		MaxSessions:       pick(fs, "max-sessions", "MAX_SESSIONS", *maxSessions, strconv.Atoi),
		IgnoreHTTPSErrors: pick(fs, "ignore-https-errors", "IGNORE_HTTPS_ERRORS", *ignoreHTTPSErrors, strconv.ParseBool),
		Headful:           pick(fs, "headful", "HEADFUL", *headful, strconv.ParseBool),
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = "http://localhost:" + cfg.Port
	}
	cfg.PublicURL = strings.TrimRight(cfg.PublicURL, "/")

	domains := pick(fs, "allowed-domains", "ALLOWED_DOMAINS", *allowedDomains, func(v string) ([]string, error) {
		return []string{v}, nil
	})
	var err error
	cfg.GlobalAllowedDomains, err = normalizeDomains(strings.FieldsFunc(strings.Join(domains, ","), func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	}))
	if err != nil {
		Fatalf("invalid allowed domains: %v", err)
	}
	return cfg
}

// pick returns the flag value when the flag was passed, otherwise the parsed
// env variable when it is set, otherwise the flag default.
func pick[T any](fs *pflag.FlagSet, flag, envKey string, flagValue T, parse func(string) (T, error)) T {
	if fs.Changed(flag) {
		return flagValue
	}
	v := strings.TrimSpace(os.Getenv(envKey))
	if v == "" {
		return flagValue
	}
	parsed, err := parse(v)
	if err != nil {
		Fatalf("invalid %s: %v", envKey, err)
	}
	return parsed
}

func parseString(v string) (string, error) {
	return v, nil
}
