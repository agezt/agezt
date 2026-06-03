// SPDX-License-Identifier: MIT

package plugin

import (
	"strings"
	"testing"
)

func TestParsePluginSpec_Valid(t *testing.T) {
	got, err := ParsePluginSpec("search=/usr/bin/agezt-search,scrape=/opt/scraper -v --depth 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Prefix != "search" || got[0].Path != "/usr/bin/agezt-search" || len(got[0].Args) != 0 {
		t.Errorf("entry 0 = %+v", got[0])
	}
	if got[1].Prefix != "scrape" || got[1].Path != "/opt/scraper" {
		t.Errorf("entry 1 = %+v", got[1])
	}
	if want := []string{"-v", "--depth", "2"}; strings.Join(got[1].Args, " ") != strings.Join(want, " ") {
		t.Errorf("entry 1 args = %v, want %v", got[1].Args, want)
	}
}

func TestParsePluginSpec_WhitespaceTolerated(t *testing.T) {
	got, err := ParsePluginSpec("  a = /bin/x ,  b=/bin/y  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].Prefix != "a" || got[0].Path != "/bin/x" || got[1].Prefix != "b" {
		t.Fatalf("got %+v", got)
	}
}

func TestParsePluginSpec_EmptyAndTrailingCommas(t *testing.T) {
	for _, spec := range []string{"", "   ", ",", " , , "} {
		got, err := ParsePluginSpec(spec)
		if err != nil {
			t.Errorf("spec %q: unexpected error %v", spec, err)
		}
		if len(got) != 0 {
			t.Errorf("spec %q: got %d entries, want 0", spec, len(got))
		}
	}
	// A trailing comma after a real entry is fine.
	got, err := ParsePluginSpec("a=/x,")
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

func TestParsePluginSpec_Errors(t *testing.T) {
	cases := map[string]string{
		"missing equals":     "search/usr/bin/x",
		"empty prefix":       "=/usr/bin/x",
		"empty path":         "search=",
		"whitespace path":    "search=   ",
		"dup in one of many": "a=/x,b=/y,a=/z",
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePluginSpec(spec); err == nil {
				t.Fatalf("spec %q: expected error, got nil", spec)
			}
		})
	}
}

func TestParsePluginSpec_DuplicatePrefixRejected(t *testing.T) {
	// The M223 bug: two entries with the same prefix used to spawn two
	// processes and emit a misleading conflict warning. Now a hard error.
	_, err := ParsePluginSpec("search=/bin/a,search=/bin/b")
	if err == nil {
		t.Fatal("expected duplicate-prefix error")
	}
	if !strings.Contains(err.Error(), "more than once") {
		t.Errorf("error = %q, want 'more than once'", err)
	}
	// Whitespace around the duplicate must not let it slip through.
	if _, err := ParsePluginSpec("x=/a, x =/b"); err == nil {
		t.Fatal("expected duplicate-prefix error despite whitespace")
	}
}

func TestParsePluginSpec_QuotedPathWithSpaces(t *testing.T) {
	// The Windows case: a plugin under "Program Files".
	got, err := ParsePluginSpec(`win="C:/Program Files/agezt-tool.exe" --verbose`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].Path != "C:/Program Files/agezt-tool.exe" {
		t.Errorf("path = %q, want the spaced path intact", got[0].Path)
	}
	if len(got[0].Args) != 1 || got[0].Args[0] != "--verbose" {
		t.Errorf("args = %v, want [--verbose]", got[0].Args)
	}
}

func TestParsePluginSpec_SingleQuotesAndQuotedArg(t *testing.T) {
	got, err := ParsePluginSpec(`a=/opt/x 'two words' z`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Path != "/opt/x" {
		t.Errorf("path = %q", got[0].Path)
	}
	want := []string{"two words", "z"}
	if strings.Join(got[0].Args, "|") != strings.Join(want, "|") {
		t.Errorf("args = %v, want %v", got[0].Args, want)
	}
	// Single-quoted path with spaces.
	got2, err := ParsePluginSpec(`b='/opt/my app/bin'`)
	if err != nil {
		t.Fatal(err)
	}
	if got2[0].Path != "/opt/my app/bin" {
		t.Errorf("path = %q, want '/opt/my app/bin'", got2[0].Path)
	}
}

func TestParsePluginSpec_UnquotedSpacesStillSplit(t *testing.T) {
	// Backward-compatibility: without quotes, spaces split as before.
	got, err := ParsePluginSpec("a=/bin/x -v --depth 2")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Path != "/bin/x" || strings.Join(got[0].Args, " ") != "-v --depth 2" {
		t.Errorf("got %+v", got[0])
	}
}

func TestParsePluginSpec_UnterminatedQuote(t *testing.T) {
	if _, err := ParsePluginSpec(`a="/bin/unclosed`); err == nil {
		t.Fatal("expected unterminated-quote error")
	}
	if _, err := ParsePluginSpec(`a="/bin/x"`); err != nil {
		t.Fatalf("a balanced quote should be fine, got %v", err)
	}
}

func TestParsePluginSpec_EmptyQuotedPathRejected(t *testing.T) {
	// `prefix=""` parses to one empty field — still an empty path.
	if _, err := ParsePluginSpec(`a=""`); err == nil {
		t.Fatal("expected empty-path error for a=\"\"")
	}
}

func TestParsePluginSpec_NoArgsHasEmptyArgs(t *testing.T) {
	got, err := ParsePluginSpec("a=/bin/x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Args) != 0 {
		t.Errorf("Args = %v, want empty for a no-arg entry", got[0].Args)
	}
}
