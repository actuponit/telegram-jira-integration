// Command bot wires adapters into use cases and starts the HTTP server.
// No business logic lives here.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/actuponit/telegram-jira-integration/internal/adapters/config"
	"github.com/actuponit/telegram-jira-integration/internal/adapters/gemini"
	"github.com/actuponit/telegram-jira-integration/internal/adapters/jira"
	"github.com/actuponit/telegram-jira-integration/internal/adapters/telegram"
	"github.com/actuponit/telegram-jira-integration/internal/platform/httpserver"
	"github.com/actuponit/telegram-jira-integration/internal/platform/logging"
)

const startupValidationTimeout = 30 * time.Second

func main() {
	logger := logging.New()

	if err := run(logger); err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), startupValidationTimeout)
	defer cancel()

	secrets, err := loadSecrets()
	if err != nil {
		return err
	}

	resolver, err := config.Load(env("ASSIGNEE_MAPPING_PATH", "configs/assignees.yaml"))
	if err != nil {
		return fmt.Errorf("load assignee mapping: %w", err)
	}

	allowlist, err := config.ParseChatAllowlist(secrets.allowedChatIDs)
	if err != nil {
		return fmt.Errorf("parse chat allowlist: %w", err)
	}

	drafter, err := gemini.New(ctx, secrets.geminiAPIKey)
	if err != nil {
		return fmt.Errorf("create gemini adapter: %w", err)
	}

	tracker := jira.New(secrets.jiraBaseURL, secrets.jiraEmail, secrets.jiraAPIToken, secrets.jiraProjectKey)
	if err := tracker.ValidateStartup(ctx); err != nil {
		return fmt.Errorf("validate jira config: %w", err)
	}

	webhook, err := telegram.New(secrets.telegramBotToken, secrets.telegramWebhookSecret, allowlist, drafter, tracker, resolver, logger)
	if err != nil {
		return fmt.Errorf("create telegram adapter: %w", err)
	}

	// The command menu is cosmetic (autocomplete in clients), so a Telegram
	// hiccup here must not take the service down.
	if err := webhook.RegisterCommands(); err != nil {
		logger.Warn("register telegram commands", "error", err)
	}

	addr := ":" + env("PORT", "8080")
	mux := httpserver.NewMux()
	mux.Handle("/webhook/telegram", webhook)
	srv := httpserver.New(addr, mux)

	logger.Info("starting server", "addr", addr)
	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("server stopped: %w", err)
	}
	return nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// secrets holds every credential this bot needs, read from env vars for
// local dev. Ticket 05 (Cloud Run deploy) wires these to Secret Manager in
// production, which Cloud Run injects as env vars too — no code change.
type secrets struct {
	geminiAPIKey          string
	jiraBaseURL           string
	jiraEmail             string
	jiraAPIToken          string
	jiraProjectKey        string
	telegramBotToken      string
	telegramWebhookSecret string

	// allowedChatIDs is the comma-separated chat allowlist. Not a secret,
	// but read the same way so a missing value fails startup rather than
	// silently defaulting to "answer nobody".
	allowedChatIDs string
}

// loadSecrets reads every required env var, collecting every missing one
// into a single error so startup fails fast with a complete picture rather
// than one var at a time.
func loadSecrets() (secrets, error) {
	var s secrets
	dest := map[string]*string{
		"GEMINI_API_KEY":          &s.geminiAPIKey,
		"JIRA_BASE_URL":           &s.jiraBaseURL,
		"JIRA_EMAIL":              &s.jiraEmail,
		"JIRA_API_TOKEN":          &s.jiraAPIToken,
		"JIRA_PROJECT_KEY":        &s.jiraProjectKey,
		"TELEGRAM_BOT_TOKEN":      &s.telegramBotToken,
		"TELEGRAM_WEBHOOK_SECRET": &s.telegramWebhookSecret,
		"TELEGRAM_ALLOWED_CHATS":  &s.allowedChatIDs,
	}

	var missing []string
	for name, ptr := range dest {
		v := os.Getenv(name)
		if v == "" {
			missing = append(missing, name)
			continue
		}
		*ptr = v
	}
	if len(missing) > 0 {
		return secrets{}, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	return s, nil
}
