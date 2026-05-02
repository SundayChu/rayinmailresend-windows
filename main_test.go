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

func TestResendRecipientsForHeadersExactRoute(t *testing.T) {
	cfg := &appConfig{values: map[string]string{
		"resend.to":                          "default@example.com",
		"resend.route.from.boss@example.com": "boss-target@gmail.com",
	}}

	recipients, route, err := resendRecipientsForHeaders(cfg, map[string]string{
		"From": "Boss <boss@example.com>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if route != "from:boss@example.com" {
		t.Fatalf("route = %q, want from:boss@example.com", route)
	}
	if len(recipients) != 1 || recipients[0] != "boss-target@gmail.com" {
		t.Fatalf("recipients = %#v, want boss-target@gmail.com", recipients)
	}
}

func TestResendRecipientsForHeadersRecipientRoute(t *testing.T) {
	cfg := &appConfig{values: map[string]string{
		"resend.to":                             "default@example.com",
		"resend.route.to.sunday@rayin.com.tw":   "sunday@gmail.com",
		"resend.route.to.daniel@rayin.com.tw":   "rain@gmail.com.tw",
		"resend.route.from.daniel@rayin.com.tw": "sender-route@example.com",
	}}

	recipients, route, err := resendRecipientsForHeaders(cfg, map[string]string{
		"From": "Daniel <daniel@rayin.com.tw>",
		"To":   "Daniel <daniel@rayin.com.tw>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if route != "to:daniel@rayin.com.tw" {
		t.Fatalf("route = %q, want to:daniel@rayin.com.tw", route)
	}
	if len(recipients) != 1 || recipients[0] != "rain@gmail.com.tw" {
		t.Fatalf("recipients = %#v, want rain@gmail.com.tw", recipients)
	}

	recipients, route, err = resendRecipientsForHeaders(cfg, map[string]string{
		"From": "Someone <someone@example.com>",
		"Cc":   "Sunday <sunday@rayin.com.tw>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if route != "to:sunday@rayin.com.tw" {
		t.Fatalf("route = %q, want to:sunday@rayin.com.tw", route)
	}
	if len(recipients) != 1 || recipients[0] != "sunday@gmail.com" {
		t.Fatalf("recipients = %#v, want sunday@gmail.com", recipients)
	}
}

func TestResendRecipientsForHeadersDomainRouteAndFallback(t *testing.T) {
	cfg := &appConfig{values: map[string]string{
		"resend.to":                        "default@example.com",
		"resend.route.from.vendor.com":     "vendor-team@gmail.com",
		"resend.route.from.example.com":    "example-team@gmail.com",
		"resend.route.from.vip.vendor.com": "vip-vendor@gmail.com",
	}}

	recipients, route, err := resendRecipientsForHeaders(cfg, map[string]string{
		"From": "Sales <sales@vip.vendor.com>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if route != "from:vip.vendor.com" {
		t.Fatalf("route = %q, want from:vip.vendor.com", route)
	}
	if len(recipients) != 1 || recipients[0] != "vip-vendor@gmail.com" {
		t.Fatalf("recipients = %#v, want vip-vendor@gmail.com", recipients)
	}

	recipients, route, err = resendRecipientsForHeaders(cfg, map[string]string{
		"From": "other@unknown.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if route != "default" {
		t.Fatalf("route = %q, want default", route)
	}
	if len(recipients) != 1 || recipients[0] != "default@example.com" {
		t.Fatalf("recipients = %#v, want default@example.com", recipients)
	}
}

func TestRouteMatchesFrom(t *testing.T) {
	tests := []struct {
		pattern string
		from    string
		want    bool
	}{
		{"boss@example.com", "Boss <boss@example.com>", true},
		{"example.com", "boss@example.com", true},
		{"@example.com", "boss@example.com", true},
		{"*@example.com", "boss@example.com", true},
		{"vendor.com", "boss@example.com", false},
	}

	for _, tt := range tests {
		if got := routeMatchesFrom(tt.pattern, tt.from); got != tt.want {
			t.Fatalf("routeMatchesFrom(%q, %q) = %v, want %v", tt.pattern, tt.from, got, tt.want)
		}
	}
}

func TestShouldSkipResendBySubject(t *testing.T) {
	cfg := &appConfig{values: map[string]string{
		"resend.skip.subject.contains": "UNIPSG(",
	}}

	skipped, reason := shouldSkipResend(cfg, map[string]string{
		"From":    "PSC.620WTS2 <PSC.620WTS2@uni-psg.com>",
		"Subject": "UNIPSG(10.72.247.14) 2026/05/02 17:00:00",
	})
	if !skipped {
		t.Fatal("expected UNIPSG notification to be skipped")
	}
	if reason == "" {
		t.Fatal("expected skip reason")
	}
}
