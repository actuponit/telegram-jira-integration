package config_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/actuponit/telegram-jira-integration/internal/adapters/config"
)

func writeFixture(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "assignees.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

func TestResolve_Hit(t *testing.T) {
	path := writeFixture(t, `
alice:
  jira_account_id: "5f8a"
  display_name: "Alice Smith"
`)

	r, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	assignee, ok := r.Resolve(context.Background(), "alice")
	if !ok {
		t.Fatalf("expected hit for alice")
	}
	if assignee.AccountID != "5f8a" || assignee.DisplayName != "Alice Smith" {
		t.Fatalf("unexpected assignee: %+v", assignee)
	}
}

func TestResolve_NormalizesHandle(t *testing.T) {
	path := writeFixture(t, `
alice:
  jira_account_id: "5f8a"
  display_name: "Alice Smith"
`)

	r, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	for _, handle := range []string{"@alice", "Alice", " alice ", "@Alice"} {
		assignee, ok := r.Resolve(context.Background(), handle)
		if !ok {
			t.Fatalf("expected hit for handle %q", handle)
		}
		if assignee.AccountID != "5f8a" {
			t.Fatalf("unexpected assignee for handle %q: %+v", handle, assignee)
		}
	}
}

func TestResolve_Miss(t *testing.T) {
	path := writeFixture(t, `
alice:
  jira_account_id: "5f8a"
  display_name: "Alice Smith"
`)

	r, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	assignee, ok := r.Resolve(context.Background(), "bob")
	if ok {
		t.Fatalf("expected miss for bob, got %+v", assignee)
	}
}

func TestResolve_EmptyHandle(t *testing.T) {
	path := writeFixture(t, `
alice:
  jira_account_id: "5f8a"
  display_name: "Alice Smith"
`)

	r, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	assignee, ok := r.Resolve(context.Background(), "")
	if ok {
		t.Fatalf("expected miss for empty handle, got %+v", assignee)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := config.Load(filepath.Join(dir, "does-not-exist.yaml"))
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestLoad_MalformedFile(t *testing.T) {
	path := writeFixture(t, "not: valid: yaml: [")

	_, err := config.Load(path)
	if err == nil {
		t.Fatalf("expected error for malformed file")
	}
}
