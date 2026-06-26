// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
)

// TestBuildAWSCredChain_WebIdentityAutoActivates: the standard EKS-injected
// env vars (AWS_WEB_IDENTITY_TOKEN_FILE + AWS_ROLE_ARN) turn on the IRSA layer
// with NO agezt-specific opt-in, and the boot-banner description reports it.
// Construction is network-free (the lookup is lazy), so no STS call happens.
func TestBuildAWSCredChain_WebIdentityAutoActivates(t *testing.T) {
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/var/run/secrets/eks.amazonaws.com/serviceaccount/token")
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/EksPodRole")
	// Keep other opt-ins from bleeding in from the host env.
	t.Setenv("AGEZT_AWS_SSO_PROFILE", "")
	t.Setenv("AGEZT_AWS_ASSUME_ROLE_ARN", "")

	_, desc := buildAWSCredChain(func(string) string { return "" })
	if !strings.Contains(desc, "web_identity=EksPodRole") {
		t.Errorf("desc should report the web_identity layer; got %q", desc)
	}
}

// TestBuildAWSCredChain_NoWebIdentityWithoutRoleArn: a token-file path alone
// (no AWS_ROLE_ARN) must NOT activate the layer — we can't assume a role
// without knowing which one.
func TestBuildAWSCredChain_NoWebIdentityWithoutRoleArn(t *testing.T) {
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/some/token")
	t.Setenv("AWS_ROLE_ARN", "")
	t.Setenv("AGEZT_AWS_SSO_PROFILE", "")
	t.Setenv("AGEZT_AWS_ASSUME_ROLE_ARN", "")

	_, desc := buildAWSCredChain(func(string) string { return "" })
	if strings.Contains(desc, "web_identity") {
		t.Errorf("web_identity must not activate without AWS_ROLE_ARN; got %q", desc)
	}
}

// TestParseAssumeRoleDurationSeconds: a missing/malformed/zero/negative value
// degrades to 0 (→ AWS default 3600 in kernel/creds), never a negative that STS
// rejects at runtime. The negative case is the bug this guards (M436).
func TestParseAssumeRoleDurationSeconds(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},            // unset → default
		{"3600", 3600},     // valid
		{"900", 900},       // valid (STS minimum)
		{"  1200  ", 1200}, // trimmed
		{"0", 0},           // explicit zero → default
		{"-5", 0},          // NEGATIVE → default (was passed to STS verbatim)
		{"-3600", 0},       // negative → default
		{"abc", 0},         // malformed → default
		{"12.5", 0},        // non-integer → default
	}
	for _, c := range cases {
		if got := parseAssumeRoleDurationSeconds(c.in); got != c.want {
			t.Errorf("parseAssumeRoleDurationSeconds(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCatalogScopedVaultLookup_SuppressesDuplicateBareVaultKeys(t *testing.T) {
	cat := &catalog.Catalog{Providers: map[string]*catalog.Provider{
		"alpha": {ID: "alpha", Env: []string{"SHARED_API_KEY", "ALPHA_API_KEY"}},
		"beta":  {ID: "beta", Env: []string{"SHARED_API_KEY"}},
	}}
	values := map[string]string{
		"SHARED_API_KEY": "legacy-key",
		"ALPHA_API_KEY":  "unique-key",
		catalog.ProviderCredentialName("alpha", "SHARED_API_KEY"): "scoped-key",
	}
	lookup := catalogScopedVaultLookup(cat, func(name string) string { return values[name] })

	if got := lookup("SHARED_API_KEY"); got != "" {
		t.Fatalf("duplicate bare vault key leaked through lookup: %q", got)
	}
	if got := lookup("ALPHA_API_KEY"); got != "unique-key" {
		t.Fatalf("unique bare vault key = %q, want unique-key", got)
	}
	if got := lookup(catalog.ProviderCredentialName("alpha", "SHARED_API_KEY")); got != "scoped-key" {
		t.Fatalf("provider-scoped vault key = %q, want scoped-key", got)
	}
}
