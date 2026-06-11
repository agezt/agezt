// SPDX-License-Identifier: MIT

package runtime

import (
	"slices"
	"testing"
)

func TestKeyedModelChain(t *testing.T) {
	keyed := func(want ...string) func(string) bool {
		return func(m string) bool { return slices.Contains(want, m) }
	}

	cases := []struct {
		name      string
		subModel  string
		chain     []string
		avail     func(string) bool
		def       string
		wantModel string
		wantChain []string
	}{
		{
			name:     "drops unkeyed primary, falls back to keyed default",
			subModel: "unkeyed-x", chain: nil, avail: keyed("good"), def: "good",
			wantModel: "good", wantChain: nil,
		},
		{
			name:     "keeps keyed primary + filters fallbacks",
			subModel: "good", chain: []string{"good", "bad", "good2"}, avail: keyed("good", "good2"), def: "good",
			wantModel: "good", wantChain: []string{"good", "good2"},
		},
		{
			name:     "single keyed model → nil chain",
			subModel: "good", chain: nil, avail: keyed("good"), def: "deflt",
			wantModel: "good", wantChain: nil,
		},
		{
			name:     "unkeyed fallback is dropped, leaving the keyed primary",
			subModel: "good", chain: []string{"good", "bad"}, avail: keyed("good"), def: "deflt",
			wantModel: "good", wantChain: nil,
		},
		{
			name:     "nothing keyed and no default → originals unchanged",
			subModel: "bad", chain: []string{"bad2"}, avail: keyed("none"), def: "",
			wantModel: "bad", wantChain: []string{"bad2"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotModel, gotChain := keyedModelChain(c.subModel, c.chain, c.avail, c.def)
			if gotModel != c.wantModel {
				t.Errorf("model = %q, want %q", gotModel, c.wantModel)
			}
			if !slices.Equal(gotChain, c.wantChain) {
				t.Errorf("chain = %v, want %v", gotChain, c.wantChain)
			}
		})
	}
}
