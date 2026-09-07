// Package httpserver is the Cloud Run entrypoint: HTTP server bootstrap
// and health checks. No business logic.
package httpserver

import (
	"net/http"
	"time"
)

// NewMux returns the HTTP mux with the health-check route registered.
func NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

// New builds an *http.Server listening on addr, serving the health check
// (and, once wired in cmd/bot, the Telegram webhook route).
func New(addr string, mux *http.ServeMux) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
