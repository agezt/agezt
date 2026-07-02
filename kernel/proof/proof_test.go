// SPDX-License-Identifier: MIT

package proof

import (
	"testing"

	"github.com/agezt/agezt/kernel/assure"
)

func TestSatisfied(t *testing.T) {
	cases := []struct {
		name string
		p    Proof
		want bool
	}{
		{
			name: "no criteria, complete verdict",
			p:    Proof{Verdict: assure.Verdict{Complete: true}},
			want: true,
		},
		{
			name: "no criteria, incomplete verdict",
			p:    Proof{Verdict: assure.Verdict{Complete: false, Gap: "not done"}},
			want: false,
		},
		{
			name: "complete verdict, all criteria met",
			p: Proof{
				Verdict:  assure.Verdict{Complete: true},
				Criteria: []Criterion{{Text: "tests pass", Met: true}, {Text: "doc updated", Met: true}},
			},
			want: true,
		},
		{
			name: "complete verdict, one criterion unmet",
			p: Proof{
				Verdict:  assure.Verdict{Complete: true},
				Criteria: []Criterion{{Text: "tests pass", Met: true}, {Text: "doc updated", Met: false}},
			},
			want: false,
		},
		{
			name: "incomplete verdict overrides met criteria",
			p: Proof{
				Verdict:  assure.Verdict{Complete: false, Gap: "verifier not convinced"},
				Criteria: []Criterion{{Text: "tests pass", Met: true}},
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.Satisfied(); got != tc.want {
				t.Fatalf("Satisfied() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUnmetCount(t *testing.T) {
	p := Proof{Criteria: []Criterion{{Met: true}, {Met: false}, {Met: false}}}
	if got := p.UnmetCount(); got != 2 {
		t.Fatalf("UnmetCount() = %d, want 2", got)
	}
	if got := (Proof{}).UnmetCount(); got != 0 {
		t.Fatalf("UnmetCount() on empty = %d, want 0", got)
	}
}

func TestCloneIsDeep(t *testing.T) {
	orig := Proof{
		Criteria: []Criterion{{Text: "a", Met: true}},
		Evidence: Evidence{Artifacts: []string{"art1"}},
	}
	cp := orig.Clone()
	cp.Criteria[0].Met = false
	cp.Evidence.Artifacts[0] = "mutated"
	if !orig.Criteria[0].Met {
		t.Fatal("Clone shares criteria backing array")
	}
	if orig.Evidence.Artifacts[0] != "art1" {
		t.Fatal("Clone shares artifact backing array")
	}
}
