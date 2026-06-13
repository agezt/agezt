// SPDX-License-Identifier: MIT

package agentgw

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestResolveTokenSecret_PersistsAcrossCalls proves the per-install signing
// secret is STABLE: the daemon (validator) and the `agt` CLI (minter) derive
// the SAME key from one base dir. This is the core of the C1 fix — without it,
// tokens minted by the CLI would never validate at the daemon.
func TestResolveTokenSecret_PersistsAcrossCalls(t *testing.T) {
	t.Setenv(TokenSecretEnv, "") // isolate from any real env override

	dir := t.TempDir()

	first, err := ResolveTokenSecret(dir)
	if err != nil {
		t.Fatalf("ResolveTokenSecret (1st): %v", err)
	}
	if len(first) < secretBytes {
		t.Fatalf("first secret too short: got %d bytes, want >= %d", len(first), secretBytes)
	}

	// A second resolution from the same dir must return the identical key
	// (read from the persisted file, not a fresh random one).
	second, err := ResolveTokenSecret(dir)
	if err != nil {
		t.Fatalf("ResolveTokenSecret (2nd): %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("ResolveTokenSecret returned DIFFERENT secrets across calls — daemon and CLI would disagree")
	}

	// The persisted file must be 0600 on POSIX (Windows ACLs don't honor the
	// Unix perm bits — os.OpenFile ignores them there — so only assert there).
	info, err := os.Stat(filepath.Join(dir, tokenSecretFile))
	if err != nil {
		t.Fatalf("stat secret file: %v", err)
	}
	if runtime.GOOS != "windows" {
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("secret file mode: got %o, want 0600", mode)
		}
	}
}

// TestResolveTokenSecret_EnvOverride confirms $AGEZT_AGENTGW_TOKEN_SECRET wins
// over the persisted file — the documented escape hatch for daemon and CLI on
// different hosts sharing one out-of-band secret.
func TestResolveTokenSecret_EnvOverride(t *testing.T) {
	const want = "operator-distributed-out-of-band-secret"
	t.Setenv(TokenSecretEnv, want)

	dir := t.TempDir()
	// Seed a persisted file first; the env var must still take precedence.
	if _, err := ResolveTokenSecret(dir); err != nil {
		t.Fatalf("seed persisted secret: %v", err)
	}

	got, err := ResolveTokenSecret(dir)
	if err != nil {
		t.Fatalf("ResolveTokenSecret: %v", err)
	}
	if string(got) != want {
		t.Errorf("env override ignored: got %q, want %q", string(got), want)
	}
}

// TestResolveTokenSecret_EmptyBaseDir returns a non-empty process-lifetime
// secret and must NOT touch the filesystem (safe default when there's nowhere
// to persist).
func TestResolveTokenSecret_EmptyBaseDir(t *testing.T) {
	t.Setenv(TokenSecretEnv, "")

	sec, err := ResolveTokenSecret("")
	if err != nil {
		t.Fatalf("ResolveTokenSecret(\"\"): %v", err)
	}
	if len(sec) < secretBytes {
		t.Errorf("empty base dir secret too short: got %d, want >= %d", len(sec), secretBytes)
	}
}

// TestResolveTokenSecret_NotHardcoded is the C1 regression guard: the resolved
// key must never equal the old public-source constant. A future refactor that
// re-introduces a hardcoded key would fail here.
func TestResolveTokenSecret_NotHardcoded(t *testing.T) {
	t.Setenv(TokenSecretEnv, "")

	sec, err := ResolveTokenSecret(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveTokenSecret: %v", err)
	}
	const old = "change-me-in-production"
	if string(sec) == old {
		t.Fatalf("resolved secret equals the hardcoded literal %q — auth is forgeable again", old)
	}
	// Two fresh base dirs must yield two DIFFERENT random keys (not a constant).
	a, _ := ResolveTokenSecret(t.TempDir())
	b, _ := ResolveTokenSecret(t.TempDir())
	if string(a) == string(b) {
		t.Error("two fresh installs produced identical secrets — secret generation is not random")
	}
}

// TestResolveTokenSecret_ConcurrentFirstRun exercises the O_EXCL first-run race:
// N callers resolving from the same not-yet-seeded dir must all converge on ONE
// key (whoever won the O_CREATE|O_EXCL race), never disagree.
func TestResolveTokenSecret_ConcurrentFirstRun(t *testing.T) {
	t.Setenv(TokenSecretEnv, "")

	dir := t.TempDir()
	const n = 16
	type res struct {
		sec []byte
		err error
	}
	ch := make(chan res, n)
	for range n {
		go func() {
			sec, err := ResolveTokenSecret(dir)
			ch <- res{sec, err}
		}()
	}
	var first []byte
	for range n {
		r := <-ch
		if r.err != nil {
			t.Fatalf("concurrent resolve errored: %v", r.err)
		}
		if len(r.sec) < secretBytes {
			t.Fatalf("short secret: %d bytes", len(r.sec))
		}
		if first == nil {
			first = r.sec
			continue
		}
		if string(first) != string(r.sec) {
			t.Fatalf("concurrent callers disagreed on the secret:\n  %x\n  %x", first, r.sec)
		}
	}
	// Sanity: ensure we didn't accidentally compare empty values.
	if strings.TrimSpace(string(first)) == "" {
		t.Fatal("converged on an empty secret")
	}
}
