package config

import (
	"strings"
	"testing"
)

func TestResolveProviderNameUsesConfiguredDefault(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.DefaultProvider = OpenRouterProviderName

	resolved, err := cfg.ResolveProviderName("")
	if err != nil {
		t.Fatalf("ResolveProviderName(\"\") error = %v", err)
	}
	if resolved != OpenRouterProviderName {
		t.Fatalf("ResolveProviderName(\"\") = %q, want %q", resolved, OpenRouterProviderName)
	}

	profile, ok := cfg.Provider("")
	if !ok {
		t.Fatal("Provider(\"\") = not found, want configured default profile")
	}
	if profile.Name != OpenRouterProviderName {
		t.Fatalf("Provider(\"\").Name = %q, want %q", profile.Name, OpenRouterProviderName)
	}
}

func TestResolveProviderNameUsesOverride(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.DefaultProvider = OpenRouterProviderName

	resolved, err := cfg.ResolveProviderName(DefaultProviderName)
	if err != nil {
		t.Fatalf("ResolveProviderName(%q) error = %v", DefaultProviderName, err)
	}
	if resolved != DefaultProviderName {
		t.Fatalf("ResolveProviderName(%q) = %q, want %q", DefaultProviderName, resolved, DefaultProviderName)
	}

	profile, ok := cfg.Provider(DefaultProviderName)
	if !ok {
		t.Fatalf("Provider(%q) = not found, want override profile", DefaultProviderName)
	}
	if profile.Name != DefaultProviderName {
		t.Fatalf("Provider(%q).Name = %q, want %q", DefaultProviderName, profile.Name, DefaultProviderName)
	}
}

func TestResolveProviderNameRejectsUnknownOverride(t *testing.T) {
	t.Parallel()

	cfg := Default()

	_, err := cfg.ResolveProviderName("missing")
	if err == nil {
		t.Fatal("ResolveProviderName(\"missing\") error = nil, want error")
	}
	if !strings.Contains(err.Error(), `provider "missing" is not configured`) {
		t.Fatalf("ResolveProviderName(\"missing\") error = %q, want unknown provider error", err)
	}

	if _, ok := cfg.Provider("missing"); ok {
		t.Fatal("Provider(\"missing\") ok = true, want false")
	}
}

func TestResolveProviderNameRejectsMissingDefaultProvider(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.DefaultProvider = OpenRouterProviderName
	delete(cfg.Providers, OpenRouterProviderName)

	_, err := cfg.ResolveProviderName("")
	if err == nil {
		t.Fatal("ResolveProviderName(\"\") error = nil, want error")
	}
	if !strings.Contains(err.Error(), `provider "openrouter" is not configured`) {
		t.Fatalf("ResolveProviderName(\"\") error = %q, want missing default provider error", err)
	}

	if _, ok := cfg.Provider(""); ok {
		t.Fatal("Provider(\"\") ok = true, want false")
	}
}
