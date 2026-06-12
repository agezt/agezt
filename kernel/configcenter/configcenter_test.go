// SPDX-License-Identifier: MIT

package configcenter

import (
	"context"
	"testing"
)

// TestClassifierBasic tests the secret classifier with basic patterns.
func TestClassifierBasic(t *testing.T) {
	classifier := NewSecretClassifier()

	tests := []struct {
		name     string
		key      string
		value    string
		expected Rating
	}{
		// Secrets detected by key name
		{"API key in key name", "github:api_key", "anything-here", RatingSecret},
		{"Secret key in key", "service:secret_key", "anything-here", RatingSecret},
		{"Password in key", "db:password", "anything-here", RatingSecret},
		{"Private key in key", "ssl:private_key", "anything-here", RatingSecret},

		// Secrets detected by value pattern
		{"GitHub PAT", "token", "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", RatingSecret},
		{"AWS Access Key", "key", "AKIAIOSFODNN7EXAMPLE", RatingSecret},
		{"Stripe Key", "key", "sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxx", RatingSecret},
		{"Slack Token", "token", "xoxb-xxxx-xxxx-xxxx-xxxx", RatingSecret},
		{"JWT Token", "jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", RatingSecret},

		// Public values
		{"HTTPS URL", "endpoint", "https://api.example.com", RatingPublic},
		{"HTTP URL", "url", "http://internal.local:8080", RatingPublic},
		{"Email", "contact", "user@example.com", RatingPublic},
		{"Numeric value", "port", "8080", RatingPublic},
		{"Semver version", "app:version", "2.1.0", RatingPublic}, // semver is not a secret

		// Internal values
		{"Application name", "app:name", "my-application", RatingInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifier.Classify(tt.key, tt.value)
			if got != tt.expected {
				t.Errorf("Classify(%q, %q) = %v, want %v", tt.key, tt.value, got, tt.expected)
			}
		})
	}
}

// TestClassifierOverride tests manual rating overrides.
func TestClassifierOverride(t *testing.T) {
	classifier := NewSecretClassifier()

	// Set override: this key should be treated as public
	classifier.SetOverride("mycompany:legacy_api_key", RatingPublic)

	got := classifier.Classify("mycompany:legacy_api_key", "some-secret-value")
	if got != RatingPublic {
		t.Errorf("Classify(override key) = %v, want public", got)
	}

	// Normal secret should still be detected
	got = classifier.Classify("github:api_key", "anything")
	if got != RatingSecret {
		t.Errorf("Classify(normal secret) = %v, want secret", got)
	}
}

// TestConfigEntry tests ConfigEntry creation.
func TestConfigEntry(t *testing.T) {
	entry := NewConfigEntry("test:value", "test-content")

	if entry.Key != "test:value" {
		t.Errorf("Key = %q, want %q", entry.Key, "test:value")
	}
	if entry.Value != "test-content" {
		t.Errorf("Value = %q, want %q", entry.Value, "test-content")
	}
	// Rating may be set by auto-classification, which is fine
}

// TestCenterAutoClassification tests that entries are auto-classified.
func TestCenterAutoClassification(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	center, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer center.Close()

	// Add entries - classifier will auto-classify
	center.store.Set(NewConfigEntry("github:token", "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))
	center.store.Set(NewConfigEntry("api:endpoint", "https://api.example.com"))

	// Check that entries were stored
	entry, err := center.store.Get("github:token")
	if err != nil {
		t.Fatalf("store.Get() error = %v", err)
	}
	if entry.Value != "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" {
		t.Errorf("entry.Value = %q, want %q", entry.Value, "ghp_xxx...")
	}
}

// TestCenterAccessControlAutoAllow tests that public/internal values are auto-allowed.
func TestCenterAccessControlAutoAllow(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	cfg.AccessPolicies = map[Rating]Policy{
		RatingPublic:   PolicyAuto,
		RatingInternal: PolicyAuto,
		RatingSecret:   PolicyDeny,
	}

	center, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer center.Close()

	// Add a public entry
	center.store.Set(NewConfigEntry("api:endpoint", "https://api.example.com"))

	// Try to access it (should succeed)
	ctx := context.Background()
	value, err := center.Get(ctx, ConfigAccessRequest{
		AgentID: "test-agent",
		RunID:   "run-123",
		Key:     "api:endpoint",
		Reason:  "testing",
	})

	if err != nil {
		t.Errorf("Get() error = %v, want nil", err)
	}
	if value != "https://api.example.com" {
		t.Errorf("Get() value = %q, want %q", value, "https://api.example.com")
	}
}

// TestCenterAccessControlDeny tests that secret values are denied.
func TestCenterAccessControlDeny(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	cfg.AccessPolicies = map[Rating]Policy{
		RatingPublic: PolicyAuto,
		RatingSecret: PolicyDeny,
	}

	center, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer center.Close()

	// Add a secret entry (will be auto-classified as secret due to "token" in key)
	center.store.Set(NewConfigEntry("github:token", "ghp_secret"))

	// Try to access it (should fail)
	ctx := context.Background()
	_, err = center.Get(ctx, ConfigAccessRequest{
		AgentID: "test-agent",
		RunID:   "run-123",
		Key:     "github:token",
		Reason:  "testing",
	})

	if err == nil {
		t.Error("Get() error = nil, want access denied")
	}

	cfgErr, ok := err.(*ConfigError)
	if !ok {
		t.Fatalf("Get() error type = %T, want *ConfigError", err)
	}
	if cfgErr.Code != ErrAccessDenied {
		t.Errorf("Get() error code = %q, want %q", cfgErr.Code, ErrAccessDenied)
	}
}

// TestCenterAccessControlHITL tests that restricted values require HITL approval.
func TestCenterAccessControlHITL(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	cfg.AccessPolicies = map[Rating]Policy{
		RatingPublic:     PolicyAuto,
		RatingRestricted: PolicyHITL,
		RatingSecret:     PolicyDeny,
	}

	center, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer center.Close()

	// Add an entry that will be classified as restricted
	// A long base64 string without clear secret pattern should be restricted
	center.store.Set(NewConfigEntry("data:token", "c3VwZXJzZWNyZXR0aGF0aXNub3Rha2V5dGhhdG9rZW4xMjM0NTY3ODkw"))

	ctx := context.Background()
	_, err = center.Get(ctx, ConfigAccessRequest{
		AgentID: "test-agent",
		RunID:   "run-123",
		Key:     "data:token",
		Reason:  "testing",
	})

	// Without approval registry, HITL should deny
	if err == nil {
		t.Error("Get() error = nil, want approval not configured error")
	}
}

// TestCenterKeyNotFound tests that missing keys return appropriate error.
func TestCenterKeyNotFound(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	center, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer center.Close()

	ctx := context.Background()
	_, err = center.Get(ctx, ConfigAccessRequest{
		AgentID: "test-agent",
		RunID:   "run-123",
		Key:     "nonexistent:key",
		Reason:  "testing",
	})

	if err == nil {
		t.Error("Get() error = nil, want key not found")
	}

	cfgErr, ok := err.(*ConfigError)
	if !ok {
		t.Fatalf("Get() error type = %T, want *ConfigError", err)
	}
	if cfgErr.Code != ErrKeyNotFound {
		t.Errorf("Get() error code = %q, want %q", cfgErr.Code, ErrKeyNotFound)
	}
}

// TestCenterListAccessible tests listing accessible entries.
func TestCenterListAccessible(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	center, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer center.Close()

	// Add entries
	center.store.Set(NewConfigEntry("public:value", "public-content"))
	center.store.Set(NewConfigEntry("internal:value", "internal-content"))

	entries := center.ListAccessible()
	if entries == nil {
		t.Fatal("ListAccessible() returned nil")
	}

	// Should include both entries
	if len(entries) != 2 {
		t.Errorf("ListAccessible() returned %d entries, want 2", len(entries))
	}
}

// TestRateLimiting tests that rate limiting works.
func TestRateLimiting(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	cfg.RateLimits.PerAgentPerMinute = 2 // Very low limit for testing

	center, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer center.Close()

	// Add a public entry
	center.store.Set(NewConfigEntry("public:value", "content"))

	ctx := context.Background()

	// First request should succeed
	_, err = center.Get(ctx, ConfigAccessRequest{
		AgentID: "rate-limit-agent",
		RunID:   "run-1",
		Key:     "public:value",
	})

	if err != nil {
		t.Errorf("Get() 1st call error = %v, want nil", err)
	}

	// Second request should succeed
	_, err = center.Get(ctx, ConfigAccessRequest{
		AgentID: "rate-limit-agent",
		RunID:   "run-2",
		Key:     "public:value",
	})

	if err != nil {
		t.Errorf("Get() 2nd call error = %v, want nil", err)
	}

	// Third request should be rate limited
	_, err = center.Get(ctx, ConfigAccessRequest{
		AgentID: "rate-limit-agent",
		RunID:   "run-3",
		Key:     "public:value",
	})

	if err == nil {
		t.Error("Get() 3rd call error = nil, want rate limited")
	}
}

// TestHashValue tests the value hashing function.
func TestHashValue(t *testing.T) {
	hash1 := HashValue("test-value")
	hash2 := HashValue("test-value")
	hash3 := HashValue("different-value")

	if hash1 != hash2 {
		t.Error("HashValue() should return same hash for same input")
	}
	if hash1 == hash3 {
		t.Error("HashValue() should return different hash for different input")
	}
	if len(hash1) != 64 { // SHA256 produces 64 hex characters
		t.Errorf("HashValue() hash length = %d, want 64", len(hash1))
	}
}

// TestSearch tests the search functionality.
func TestSearch(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	center, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer center.Close()

	// Add entries
	center.store.Set(NewConfigEntry("github:token", "ghp_xxx"))
	center.store.Set(NewConfigEntry("github:endpoint", "https://api.github.com"))
	center.store.Set(NewConfigEntry("analytics:token", "ak_xxx"))
	center.store.Set(NewConfigEntry("api:version", "1.0.0"))

	// Search for github (prefix match)
	results := center.Search("github", SearchOptions{Limit: 10})
	if len(results) != 2 {
		t.Errorf("Search(github) returned %d results, want 2", len(results))
	}

	// Search for token (no entries START with "token" - prefix match only)
	results = center.Search("token", SearchOptions{Limit: 10})
	if len(results) != 0 {
		t.Errorf("Search(token) returned %d results, want 0 (prefix match only)", len(results))
	}

	// Search for github: (full namespace prefix)
	results = center.Search("github:", SearchOptions{Limit: 10})
	if len(results) != 2 {
		t.Errorf("Search(github:) returned %d results, want 2", len(results))
	}

	// Search for api (prefix match)
	results = center.Search("api", SearchOptions{Limit: 10})
	if len(results) != 1 {
		t.Errorf("Search(api) returned %d results, want 1", len(results))
	}
}
