// SPDX-License-Identifier: MIT

package runtime

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/taste"
)

func TestInjectTastePrependsExemplars(t *testing.T) {
	system := "You are a helpful agent."
	got := injectTaste(system, []taste.Exemplar{
		{Title: "Good PR summary", Body: "One line what, one line why."},
		{Title: "House voice", Body: "Terse, concrete, no hedging."},
	})
	if !strings.Contains(got, "What good looks like") {
		t.Fatalf("missing header:\n%s", got)
	}
	if !strings.Contains(got, "Good PR summary") || !strings.Contains(got, "House voice") {
		t.Fatalf("missing exemplar titles:\n%s", got)
	}
	// The original system prompt is preserved after the exemplars.
	if !strings.HasSuffix(got, system) {
		t.Fatalf("system prompt not preserved at end:\n%s", got)
	}
}
