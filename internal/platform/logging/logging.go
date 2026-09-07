// Package logging emits structured JSON logs to stdout for Cloud Logging.
package logging

import (
	"log/slog"
	"os"
)

// New returns a logger that emits structured JSON to stdout.
func New() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil))
}
