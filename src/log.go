package src

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

// ServerLogID is used in place of a session id for logs not tied to a session.
const ServerLogID = "server"

// logWriter prefixes every log line with the time, e.g. "sep 23 21:04".
type logWriter struct {
	out io.Writer
}

func (w logWriter) Write(p []byte) (int, error) {
	ts := strings.ToLower(time.Now().Format("Jan 2 15:04"))
	if _, err := fmt.Fprintf(w.out, "%s %s", ts, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func SetupLogging() {
	log.SetFlags(0)
	log.SetOutput(logWriter{out: os.Stderr})
}

// Logf logs a message as "<time> [id] message".
func Logf(id, format string, args ...any) {
	log.Printf("[%s] %s", id, fmt.Sprintf(format, args...))
}

func Fatalf(format string, args ...any) {
	log.Fatalf("[%s] %s", ServerLogID, fmt.Sprintf(format, args...))
}
