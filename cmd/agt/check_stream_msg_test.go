// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

// The streaming-unsupported message names the family and points at the plain
// check, and no longer carries the stale "only anthropic is wired / lands in
// M1.q.x" claim that every first-party family has since outgrown (M267).
func TestStreamingUnsupportedMessage(t *testing.T) {
	msg := streamingUnsupportedMessage("acme")

	if !strings.Contains(msg, `"acme"`) {
		t.Errorf("message %q should name the family", msg)
	}
	if !strings.Contains(msg, "--stream") {
		t.Errorf("message %q should point at running without --stream", msg)
	}
	for _, stale := range []string{"M1.q", "only wires anthropic", "land in", "does not yet support"} {
		if strings.Contains(msg, stale) {
			t.Errorf("message %q still carries the stale fragment %q", msg, stale)
		}
	}
}
