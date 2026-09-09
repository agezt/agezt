// SPDX-License-Identifier: MIT

package keys

import (
	"strings"
	"testing"
)

// Integration tests (live daemon) live in cmd/agt — the package
// owns only the pure argument-parsing branches. The four
// handlers and the dispatch share a tight set of failure modes
// (unknown subcommand, missing --provider value, missing
// positional args) and those are what the unit tests cover.

func TestRun_NoArgsExitsTwo(t *testing.T) {
	var out, errb strings.Builder
	if code := Run(nil, &out, &errb); code != 2 {
		t.Errorf("Run(nil) returned %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "subcommand required") {
		t.Errorf("expected subcommand-required message, got: %q", errb.String())
	}
}

func TestRun_HelpExitsZero(t *testing.T) {
	var out, errb strings.Builder
	if code := Run([]string{"--help"}, &out, &errb); code != 0 {
		t.Errorf("Run(--help) returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "list") {
		t.Errorf("expected list subcommand in help, got: %q", out.String())
	}
}

func TestRun_UnknownSubcommandExitsTwo(t *testing.T) {
	var out, errb strings.Builder
	if code := Run([]string{"--bogus"}, &out, &errb); code != 2 {
		t.Errorf("Run(--bogus) returned %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "unknown subcommand") {
		t.Errorf("expected unknown-subcommand message, got: %q", errb.String())
	}
}

func TestRun_AliasAccepted(t *testing.T) {
	// "ls" is an alias for "list". With a non-empty args list
	// (so listCmd is reached and not the "subcommand required"
	// arm), the dispatch should accept it; the dial will fail
	// because there's no daemon in unit-test land, returning
	// exit 1, NOT 2 (which would mean the dispatch rejected
	// the alias).
	var out, errb strings.Builder
	code := Run([]string{"ls", "ANTHROPIC_API_KEY"}, &out, &errb)
	if code == 2 {
		t.Errorf("Run(ls, …) returned 2, dispatch should accept the alias; stderr=%q", errb.String())
	}
	if code != 1 {
		t.Logf("Run(ls, …) returned %d, expected 1 from a missing-daemon dial", code)
	}
}

func TestProviderFlag_BothSyntaxesAndDefaults(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantProv   string
		wantRest   []string
	}{
		{"no flag", []string{"ENV", "label"}, "", []string{"ENV", "label"}},
		{"--provider X", []string{"--provider", "anthropic", "ENV", "label"}, "anthropic", []string{"ENV", "label"}},
		{"--provider=X", []string{"--provider=anthropic", "ENV", "label"}, "anthropic", []string{"ENV", "label"}},
		{"flag in middle", []string{"ENV", "--provider", "anthropic", "label"}, "anthropic", []string{"ENV", "label"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var errb strings.Builder
			p, rest, ok := providerFlag(c.args, &errb)
			if !ok {
				t.Errorf("providerFlag returned !ok, errb=%q", errb.String())
			}
			if p != c.wantProv {
				t.Errorf("provider = %q, want %q", p, c.wantProv)
			}
			if got, want := strings.Join(rest, "|"), strings.Join(c.wantRest, "|"); got != want {
				t.Errorf("rest = %q, want %q", got, want)
			}
		})
	}
}

func TestProviderFlag_MissingValueExitsNotOK(t *testing.T) {
	var errb strings.Builder
	_, _, ok := providerFlag([]string{"--provider"}, &errb)
	if ok {
		t.Error("providerFlag(--provider) returned ok, expected !ok")
	}
	if !strings.Contains(errb.String(), "needs a value") {
		t.Errorf("expected needs-a-value message, got: %q", errb.String())
	}
}

func TestKeyRequest_ScopedVsUnscoped(t *testing.T) {
	scoped := keyRequest("anthropic", "ANTHROPIC_API_KEY")
	if scoped["provider"] != "anthropic" {
		t.Errorf("scoped missing provider: %v", scoped)
	}
	if scoped["env"] != "ANTHROPIC_API_KEY" {
		t.Errorf("scoped missing env: %v", scoped)
	}
	unscoped := keyRequest("", "ANTHROPIC_API_KEY")
	if _, hasProvider := unscoped["provider"]; hasProvider {
		t.Errorf("unscoped should not have provider: %v", unscoped)
	}
}

func TestTargetLabel_ScopedVsUnscoped(t *testing.T) {
	if got := targetLabel("anthropic", "ANTHROPIC_API_KEY"); got != "anthropic/ANTHROPIC_API_KEY" {
		t.Errorf("scoped = %q", got)
	}
	if got := targetLabel("", "ANTHROPIC_API_KEY"); got != "ANTHROPIC_API_KEY" {
		t.Errorf("unscoped = %q", got)
	}
}

func TestFlagSnippet_EmptyOrScoped(t *testing.T) {
	if got := flagSnippet(""); got != "" {
		t.Errorf("empty = %q", got)
	}
	if got := flagSnippet("anthropic"); got != "--provider anthropic " {
		t.Errorf("scoped = %q", got)
	}
}
