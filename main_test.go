package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseProperties(t *testing.T) {
	values := map[string]string{}
	input := []byte("\uFEFF# comment\nname = value\nempty=\n; ignored\n")

	if err := parseProperties(input, values); err != nil {
		t.Fatalf("parseProperties returned error: %v", err)
	}
	if values["name"] != "value" {
		t.Fatalf("name = %q, want value", values["name"])
	}
	if _, ok := values["empty"]; !ok {
		t.Fatal("empty key was not stored")
	}
}

func TestLoadConfigDefaultsAndRelativePaths(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.properties")
	if err := os.WriteFile(configPath, []byte("gmail.credentials.file=secrets\\client.json\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Clean(filepath.Join(dir, "secrets", "client.json"))
	if got := cfg.path("gmail.credentials.file"); got != want {
		t.Fatalf("credentials path = %q, want %q", got, want)
	}
	if got := cfg.get("gmail.user.id"); got != "me" {
		t.Fatalf("default gmail.user.id = %q, want me", got)
	}
}

func TestTokenExpiryFormats(t *testing.T) {
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	token := &oauthToken{AccessToken: "token", Expiry: future}
	if token.needsRefresh() {
		t.Fatal("future RFC3339 token should not need refresh")
	}

	pastMillis := time.Now().Add(-time.Hour).UnixMilli()
	token = &oauthToken{AccessToken: "token", ExpiryDate: pastMillis}
	if !token.needsRefresh() {
		t.Fatal("past expiry_date token should need refresh")
	}
}
