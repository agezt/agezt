// SPDX-License-Identifier: MIT

package boardapi

import "testing"

// TestTheWildcardIsTheWireValue: the wildcard recipient is read BY VALUE —
// the persisted board file stores it, the SDK's mailbox filter compares it,
// the daemon's boot replay special-cases it and the Web UI shows it. So the
// VALUE is a contract of its own, not an implementation detail: it must stay
// "*", and a change to it would silently break every reader that knows the
// value and not the constant.
func TestTheWildcardIsTheWireValue(t *testing.T) {
	if Everyone != "*" {
		t.Fatalf("the wildcard recipient = %q, want %q — it is read by value", Everyone, "*")
	}
}
