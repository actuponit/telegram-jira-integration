package domain_test

import (
	"testing"

	"github.com/actuponit/telegram-jira-integration/internal/domain"
)

func TestParseApp_ValidValues(t *testing.T) {
	for _, want := range []domain.App{domain.AppMelaApp, domain.AppMerchantApp, domain.AppBackend} {
		got, err := domain.ParseApp(string(want))
		if err != nil {
			t.Fatalf("ParseApp(%q): unexpected error: %v", want, err)
		}
		if got != want {
			t.Fatalf("ParseApp(%q) = %q, want %q", want, got, want)
		}
	}
}

func TestParseApp_UnknownValue(t *testing.T) {
	if _, err := domain.ParseApp("Mobile App"); err == nil {
		t.Fatal("expected error for unknown app value, got nil")
	}
}
