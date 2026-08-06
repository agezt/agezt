// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
)

func TestIsLoopback_ClassifiesExposureCorrectly(t *testing.T) {
	// isLoopback drives the "reachable beyond localhost" exposure warning shown
	// when the web UI / control plane / REST API binds to a public address. A
	// regression that classified 0.0.0.0 or an empty host as loopback would
	// silently suppress the warning and let an operator expose the daemon. Pin the
	// security-critical cases.
	loopback := []string{
		"127.0.0.1:8800", "localhost:8800", "[::1]:8800",
		"127.0.0.1", "127.0.0.53:8800", "::1",
	}
	exposed := []string{
		"0.0.0.0:8800", // binds every interface — the classic mistake
		":8800",        // empty host = every interface
		"0.0.0.0",
		"192.168.1.5:8800", // LAN
		"10.0.0.1:8800",    // private
		"203.0.113.7:8800", // public
		"example.com:8800", // hostname (conservatively not loopback)
		"",
	}
	for _, a := range loopback {
		if !isLoopback(a) {
			t.Errorf("isLoopback(%q) = false, want true (loopback-only bind)", a)
		}
	}
	for _, a := range exposed {
		if isLoopback(a) {
			t.Errorf("isLoopback(%q) = true, want false (reachable beyond localhost)", a)
		}
	}
}

func TestTunnelTargetDefaultsToLiveWebUI(t *testing.T) {
	t.Setenv(brand.EnvPrefix+"TUNNEL_TARGET", "")
	t.Setenv(brand.EnvPrefix+"WEB_ADDR", "")
	t.Setenv(brand.EnvPrefix+"REST_ADDR", "127.0.0.1:8800")

	got := tunnelTargetFromEnv(webUISurface{localURL: "http://127.0.0.1:8787"})
	if got != "http://127.0.0.1:8787" {
		t.Fatalf("target = %q, want live Web UI", got)
	}
}

func TestTunnelTargetFallsBackToRESTWhenWebUIDisabled(t *testing.T) {
	t.Setenv(brand.EnvPrefix+"TUNNEL_TARGET", "")
	t.Setenv(brand.EnvPrefix+"WEB_ADDR", "off")
	t.Setenv(brand.EnvPrefix+"REST_ADDR", ":8800")

	got := tunnelTargetFromEnv(webUISurface{})
	if got != "http://127.0.0.1:8800" {
		t.Fatalf("target = %q, want REST loopback URL", got)
	}
}

func TestURLWithToken(t *testing.T) {
	got := urlWithToken("https://demo.trycloudflare.com/path?x=1", "abc123")
	if !strings.Contains(got, "token=abc123") || !strings.Contains(got, "x=1") {
		t.Fatalf("tokened url = %q, want existing query plus token", got)
	}
	if host := publicURLHost(got); host != "demo.trycloudflare.com" {
		t.Fatalf("host = %q, want demo.trycloudflare.com", host)
	}
}

func TestTunnelPublicURLUsesPasswordLoginWhenAvailable(t *testing.T) {
	base := "https://demo.trycloudflare.com/"
	cases := []struct {
		name string
		web  webUISurface
		want string
	}{
		{
			name: "no password stays tokened",
			web:  webUISurface{token: "tok"},
			want: "token=tok",
		},
		{
			name: "password without strict prints login url",
			web:  webUISurface{token: "tok", passwordOn: true},
			want: base,
		},
		{
			name: "strict password needs tokened url",
			web:  webUISurface{token: "tok", passwordOn: true, passwordStrict: true},
			want: "token=tok",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tunnelPublicURL(base, tc.web, true)
			if tc.want == base {
				if got != base {
					t.Fatalf("public url = %q, want plain password login URL", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("public url = %q, want %q", got, tc.want)
			}
		})
	}
	if got := tunnelPublicURL(base, webUISurface{token: "tok", passwordOn: true}, false); got != base {
		t.Fatalf("non-Web UI tunnel URL = %q, want untouched URL", got)
	}
}

// TestWriteAPIListenToken is the regression guard for VULN banner-token-leak:
// the helper must write the FULL token to a 0600 file under baseDir and return
// only a short prefix for surfacing in the boot banner. If this test ever
// returns the full token in `prefix` or writes world/group-readable perms,
// the banner leak is back.
func TestWriteAPIListenToken(t *testing.T) {
	baseDir := t.TempDir()
	const full = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // 64 hex
	prefix, err := writeAPIListenToken(baseDir, "openai.token", full)
	if err != nil {
		t.Fatalf("writeAPIListenToken: %v", err)
	}
	// Prefix must be a short slice of the full token, never the full thing.
	if prefix == full {
		t.Fatalf("prefix equals full token: %q", prefix)
	}
	if strings.Contains(prefix, full[4:len(full)-4]) {
		t.Fatalf("prefix %q contains interior of token (likely full leak)", prefix)
	}
	// Format: first 4 + "…" + last 4.
	want := full[:4] + "…" + full[len(full)-4:]
	if prefix != want {
		t.Errorf("prefix = %q, want %q", prefix, want)
	}
	// File must contain the full token (operator reads it from disk).
	got, rerr := os.ReadFile(filepath.Join(baseDir, "openai.token"))
	if rerr != nil {
		t.Fatalf("read token file: %v", rerr)
	}
	if strings.TrimSpace(string(got)) != full {
		t.Errorf("token file content = %q, want %q", string(got), full)
	}
	// File perms: 0600 owner-only. Skip the check on Windows where
	// syscall.Umask is unreliable and the per-user base dir is the real
	// boundary; unix is where the 0600 actually bites.
	if stdruntime.GOOS != "windows" {
		st, serr := os.Stat(filepath.Join(baseDir, "openai.token"))
		if serr != nil {
			t.Fatalf("stat token file: %v", serr)
		}
		if mode := st.Mode().Perm(); mode != 0o600 {
			t.Errorf("token file mode = %o, want 0600", mode)
		}
	}
	// Negative: empty inputs must error rather than write a bad file.
	if _, err := writeAPIListenToken("", "x", full); err == nil {
		t.Errorf("empty baseDir: expected error, got nil")
	}
	if _, err := writeAPIListenToken(baseDir, "x", ""); err == nil {
		t.Errorf("empty token: expected error, got nil")
	}
}
