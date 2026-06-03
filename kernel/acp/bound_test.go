// SPDX-License-Identifier: MIT

package acp

import (
	"io"
	"strings"
	"testing"
)

// A single JSON-RPC message larger than maxMessageBytes is rejected rather than
// buffered into memory — the per-message bound json.Decoder lacked (M257).
func TestScanMessage_BoundsOversizedMessage(t *testing.T) {
	huge := `{"jsonrpc":"2.0","method":"x","params":"` + strings.Repeat("a", maxMessageBytes) + `"}` + "\n"
	sc := newBoundedScanner(strings.NewReader(huge))
	var v map[string]any
	err := scanMessage(sc, &v)
	if err == nil || err == io.EOF {
		t.Fatalf("oversized message should error, got %v", err)
	}
}

// A normal message is read, leading blank lines are skipped, and a clean end of
// stream returns io.EOF.
func TestScanMessage_ReadsNormalAndSkipsBlanks(t *testing.T) {
	input := "\n\n" + `{"method":"ping"}` + "\n"
	sc := newBoundedScanner(strings.NewReader(input))

	var v struct {
		Method string `json:"method"`
	}
	if err := scanMessage(sc, &v); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if v.Method != "ping" {
		t.Errorf("method = %q, want ping", v.Method)
	}
	if err := scanMessage(sc, &v); err != io.EOF {
		t.Errorf("expected io.EOF at end of stream, got %v", err)
	}
}

// A message exactly at the cap still reads (the bound rejects only what exceeds
// it), confirming we don't reject legitimate large-but-valid messages.
func TestScanMessage_AtCapReads(t *testing.T) {
	pad := maxMessageBytes - len(`{"method":"ping","params":""}`) - 1
	if pad < 0 {
		t.Skip("cap too small for this construction")
	}
	line := `{"method":"ping","params":"` + strings.Repeat("a", pad) + `"}` + "\n"
	sc := newBoundedScanner(strings.NewReader(line))
	var v struct {
		Method string `json:"method"`
	}
	if err := scanMessage(sc, &v); err != nil {
		t.Fatalf("at-cap message should read: %v", err)
	}
	if v.Method != "ping" {
		t.Errorf("method = %q, want ping", v.Method)
	}
}
