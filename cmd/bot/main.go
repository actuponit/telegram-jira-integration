// Command bot wires adapters into use cases and starts the HTTP server.
// No business logic lives here.
package main

import (
	"os"

	"github.com/actuponit/telegram-jira-integration/internal/platform/httpserver"
	"github.com/actuponit/telegram-jira-integration/internal/platform/logging"
)

func main() {
	logger := logging.New()

	addr := ":" + port()
	mux := httpserver.NewMux()
	srv := httpserver.New(addr, mux)

	logger.Info("starting server", "addr", addr)
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8080"
}
