// SPDX-License-Identifier: MIT

package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTier(t *testing.T) {
	tests := []struct {
		tier Tier
		name string
		ok   bool
	}{
		{TierPublic, "public", true},
		{TierUser, "user", true},
		{TierAdmin, "admin", true},
		{Tier(99), "unknown", false},
	}
	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.name {
			t.Errorf("Tier(%d).String() = %q, want %q", tt.tier, got, tt.name)
		}
		if got := tt.tier.Valid(); got != tt.ok {
			t.Errorf("Tier(%d).Valid() = %v, want %v", tt.tier, got, tt.ok)
		}
	}
}

func TestStaticVerifierAuthority(t *testing.T) {
	v := NewStaticVerifier("admin-secret", "", "user-secret", "sse-secret")
	tests := []struct {
		name      string
		presented string
		required  Tier
		want      bool
	}{
		{"public needs no token", "", TierPublic, true},
		{"admin reaches admin", "admin-secret", TierAdmin, true},
		{"admin reaches user", "admin-secret", TierUser, true},
		{"user reaches user", "user-secret", TierUser, true},
		{"sse reaches user", "sse-secret", TierUser, true},
		{"user cannot reach admin", "user-secret", TierAdmin, false},
		{"blank fails closed", "", TierUser, false},
		{"wrong token", "wrong", TierUser, false},
		{"unknown tier fails closed", "admin-secret", Tier(99), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := v.Authorize(tt.presented, tt.required); got != tt.want {
				t.Fatalf("Authorize(%q, %s) = %v, want %v", tt.presented, tt.required, got, tt.want)
			}
		})
	}
}

func TestStaticVerifierEmptyAndNilFailClosed(t *testing.T) {
	var nilVerifier *StaticVerifier
	if nilVerifier.Authorize("anything", TierUser) {
		t.Fatal("nil verifier authorized a protected operation")
	}
	if NewStaticVerifier("").Authorize("", TierAdmin) {
		t.Fatal("empty verifier authorized an empty admin credential")
	}
	if NewStaticVerifier("", "").Authorize("anything", TierUser) {
		t.Fatal("empty verifier authorized a user credential")
	}
}

func TestWriteTokenFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "auth")
	token := "0123456789abcdef0123456789abcdef"
	prefix, err := WriteTokenFile(dir, "openai.token", token)
	if err != nil {
		t.Fatalf("WriteTokenFile: %v", err)
	}
	if prefix != "0123…cdef" {
		t.Fatalf("prefix = %q, want %q", prefix, "0123…cdef")
	}
	path := filepath.Join(dir, "openai.token")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if string(got) != token+"\n" {
		t.Fatalf("token file = %q, want token plus newline", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat token file: %v", err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("token file mode = %o, want 600", mode)
		}
	}

	replacement := "fedcba9876543210fedcba9876543210"
	if _, err := WriteTokenFile(dir, "openai.token", replacement); err != nil {
		t.Fatalf("replace token file: %v", err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replaced token file: %v", err)
	}
	if string(got) != replacement+"\n" {
		t.Fatalf("replaced token file = %q, want replacement plus newline", got)
	}
}

func TestWriteTokenFileRejectsUnsafeInputs(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name     string
		baseDir  string
		filename string
		token    string
	}{
		{"blank directory", "", "token", "secret"},
		{"blank filename", dir, "", "secret"},
		{"dot filename", dir, ".", "secret"},
		{"parent filename", dir, "..", "secret"},
		{"slash traversal", dir, "../token", "secret"},
		{"backslash traversal", dir, `..\token`, "secret"},
		{"nested filename", dir, "nested/token", "secret"},
		{"absolute filename", dir, filepath.Join(dir, "token"), "secret"},
		{"blank token", dir, "token", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := WriteTokenFile(tt.baseDir, tt.filename, tt.token); err == nil {
				t.Fatal("WriteTokenFile accepted unsafe input")
			}
		})
	}
}

func TestTokenPrefixDoesNotRevealShortCredentials(t *testing.T) {
	if got := TokenPrefix("12345678"); got != "" {
		t.Fatalf("short token prefix = %q, want empty", got)
	}
	if got := TokenPrefix("123456789"); got != "1234…6789" {
		t.Fatalf("long token prefix = %q", got)
	}
}
