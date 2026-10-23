// SPDX-License-Identifier: MIT

package builtinchannels

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/pulseapi"
)

// TestTheBriefIsFormattedForTheOperator: the message the operator reads in
// every chat channel is built here — a brief with a body is rendered with the
// body under the title, one without is just the title. The format is the
// operator's contract, so it is pinned.
func TestTheBriefIsFormattedForTheOperator(t *testing.T) {
	got := formatBrief(pulseapi.Brief{Title: "the fleet is quiet", Disposition: pulseapi.DispNotify})
	if got != "📣 the fleet is quiet" {
		t.Errorf("the brief message = %q", got)
	}

	got = formatBrief(pulseapi.Brief{
		Title:       "three things",
		Disposition: pulseapi.DispDigest,
		Body:        "runs:\n  - a",
	})
	if !strings.HasPrefix(got, "📣 three things\n") {
		t.Errorf("the brief message = %q", got)
	}
	if !strings.Contains(got, "runs:\n  - a") {
		t.Errorf("the body must survive the formatting, got %q", got)
	}
}
