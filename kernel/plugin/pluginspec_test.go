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

func TestParsePluginSpec_NoArgsHasEmptyArgs(t *testing.T) {
	got, err := ParsePluginSpec("a=/bin/x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Args) != 0 {
		t.Errorf("Args = %v, want empty for a no-arg entry", got[0].Args)
	}
}
