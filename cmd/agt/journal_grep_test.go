// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdJournalGrep_HelpExitsCleanly — pure flag parsing.
func TestCmdJournalGrep_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdJournalGrep([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	for _, want := range []string{"<pattern>", "--kind", "--subject", "--actor", "--correlation", "--limit"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q; got %q", want, out.String())
		}
	}
}

// TestCmdJournalGrep_RequiresAtLeastOneConstraint — `agt journal
// grep` with no pattern and no filter would be `journal tail`,
// so refuse and tell the operator to use that instead.
func TestCmdJournalGrep_RequiresAtLeastOneConstraint(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdJournalGrep(nil, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "provide a pattern or") {
		t.Errorf("stderr should explain requirement; got %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "journal tail") {
		t.Errorf("stderr should point at `journal tail` as the alternative; got %q", errOut.String())
	}
}

// TestCmdJournalGrep_RejectsSecondPositional — only one bare
// pattern; subsequent bare args are typos that would otherwise
// silently swap pattern values.
func TestCmdJournalGrep_RejectsSecondPositional(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdJournalGrep([]string{"first", "second"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("stderr should reject second positional; got %q", errOut.String())
	}
}

// TestCmdJournalGrep_FlagNeedsValue catches `agt journal grep --kind`
// without an arg — would otherwise read the next arg or pass empty.
func TestCmdJournalGrep_FlagNeedsValue(t *testing.T) {
	for _, flag := range []string{"--kind", "--subject", "--actor", "--correlation", "--limit"} {
		var out, errOut bytes.Buffer
		code := cmdJournalGrep([]string{flag}, &out, &errOut)
		if code != 2 {
			t.Errorf("%s: exit=%d want 2", flag, code)
		}
		if !strings.Contains(errOut.String(), "needs a value") &&
			!strings.Contains(errOut.String(), "must be a positive") {
			t.Errorf("%s: stderr should explain; got %q", flag, errOut.String())
		}
	}
}

// TestCmdJournalGrep_LimitMustBePositive — strconv.Atoi accepts 0
// and negatives, but limit<1 is operator error.
func TestCmdJournalGrep_LimitMustBePositive(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdJournalGrep([]string{"--limit", "0"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "positive integer") {
		t.Errorf("stderr should explain; got %q", errOut.String())
	}
}
