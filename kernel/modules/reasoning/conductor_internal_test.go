// SPDX-License-Identifier: MIT

package reasoning

import (
	"strings"
	"testing"
)

func TestParseVerdict(t *testing.T) {
	cases := []struct {
		in       string
		verdict  string
		reasonHy string // substring expected in reason
	}{
		{"PASS: looks good", "pass", "looks good"},
		{"FAIL: missing edge case", "fail", "missing edge case"},
		{"pass", "pass", ""},
		{"## PASS\nclean", "pass", "clean"},
		{"The answer is wrong", "fail", "wrong"},
	}
	for _, c := range cases {
		v, r := parseVerdict(c.in)
		if v != c.verdict {
			t.Errorf("parseVerdict(%q) verdict=%q want %q", c.in, v, c.verdict)
		}
		if c.reasonHy != "" && !strings.Contains(strings.ToLower(r), c.reasonHy) {
			t.Errorf("parseVerdict(%q) reason=%q want substring %q", c.in, r, c.reasonHy)
		}
	}
}

func TestParseRoleBriefs(t *testing.T) {
	briefs := parseRoleBriefs("THINKER: plan it\nmore plan\nWORKER: build it\nVERIFIER: check it")
	if briefs[conductorRoleThinker] != "plan it\nmore plan" {
		t.Errorf("thinker brief = %q", briefs[conductorRoleThinker])
	}
	if briefs[conductorRoleWorker] != "build it" || briefs[conductorRoleVerifier] != "check it" {
		t.Errorf("briefs = %v", briefs)
	}
	if len(parseRoleBriefs("no labels here")) != 0 {
		t.Error("unlabelled plan should yield no briefs")
	}
}

func TestExtractRunnableCode(t *testing.T) {
	lang, code, ok := extractRunnableCode("here:\n```python\nprint(1)\n```\ndone")
	if !ok || lang != "python" || code != "print(1)" {
		t.Errorf("got (%q,%q,%v)", lang, code, ok)
	}
	if _, _, ok := extractRunnableCode("```text\nhello\n```"); ok {
		t.Error("non-runnable language should not match")
	}
	if _, _, ok := extractRunnableCode("no fence at all"); ok {
		t.Error("no code block should not match")
	}
	// js/ts aliases normalise.
	if l, _, ok := extractRunnableCode("```js\nconsole.log(1)\n```"); !ok || l != "javascript" {
		t.Errorf("js alias → %q ok=%v", l, ok)
	}
	if l, _, ok := extractRunnableCode("```ts\nlet x=1\n```"); !ok || l != "typescript" {
		t.Errorf("ts alias → %q ok=%v", l, ok)
	}
}
