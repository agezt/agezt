// SPDX-License-Identifier: MIT

package pulse

import "testing"

// TestRoute_FullDispositionDialQuietHoursMatrix locks in Route — the pure
// SPEC-03 §4.3/§6.3 decision that turns (dial, disposition, quiet-hours) into a
// delivery. It is the function that decides what actually reaches the operator
// and what is allowed to break quiet hours, but until now was only exercised
// incidentally by two engine integration tests. A regression here (e.g. a
// digest leaking through quiet hours, or the Quiet dial letting a notify ping)
// would be a real anti-annoyance/safety failure with no direct guard.
//
// The table is the complete 5 dispositions × 3 dials × {quiet on/off} matrix.
func TestRoute_FullDispositionDialQuietHoursMatrix(t *testing.T) {
	cases := []struct {
		disp  Disposition
		dial  Dial
		quiet bool
		want  Delivery
	}{
		// alert always sends now — every dial, even during quiet hours.
		{DispAlert, DialQuiet, false, DeliverNow},
		{DispAlert, DialBalanced, false, DeliverNow},
		{DispAlert, DialChatty, false, DeliverNow},
		{DispAlert, DialQuiet, true, DeliverNow},
		{DispAlert, DialBalanced, true, DeliverNow},
		{DispAlert, DialChatty, true, DeliverNow},

		// act behaves like alert for delivery (breaks through everything).
		{DispAct, DialQuiet, false, DeliverNow},
		{DispAct, DialBalanced, false, DeliverNow},
		{DispAct, DialChatty, false, DeliverNow},
		{DispAct, DialQuiet, true, DeliverNow},
		{DispAct, DialBalanced, true, DeliverNow},
		{DispAct, DialChatty, true, DeliverNow},

		// notify: Quiet downgrades to digest; balanced/chatty send now —
		// but quiet hours holds those "now" sends to the digest instead.
		{DispNotify, DialQuiet, false, DeliverDigest},
		{DispNotify, DialBalanced, false, DeliverNow},
		{DispNotify, DialChatty, false, DeliverNow},
		{DispNotify, DialQuiet, true, DeliverDigest},
		{DispNotify, DialBalanced, true, DeliverDigest}, // quiet hours holds it
		{DispNotify, DialChatty, true, DeliverDigest},   // quiet hours holds it

		// digest: Quiet drops it, balanced digests, chatty surfaces now —
		// and quiet hours holds the chatty "now" to the digest.
		{DispDigest, DialQuiet, false, DeliverDrop},
		{DispDigest, DialBalanced, false, DeliverDigest},
		{DispDigest, DialChatty, false, DeliverNow},
		{DispDigest, DialQuiet, true, DeliverDrop},
		{DispDigest, DialBalanced, true, DeliverDigest},
		{DispDigest, DialChatty, true, DeliverDigest}, // quiet hours holds it

		// drop: never sent, regardless of dial or quiet hours.
		{DispDrop, DialQuiet, false, DeliverDrop},
		{DispDrop, DialBalanced, false, DeliverDrop},
		{DispDrop, DialChatty, false, DeliverDrop},
		{DispDrop, DialQuiet, true, DeliverDrop},
		{DispDrop, DialBalanced, true, DeliverDrop},
		{DispDrop, DialChatty, true, DeliverDrop},
	}
	for _, c := range cases {
		got := Route(c.dial, c.disp, c.quiet)
		if got != c.want {
			t.Errorf("Route(dial=%s, disp=%s, quiet=%v) = %s, want %s",
				c.dial, c.disp, c.quiet, got, c.want)
		}
	}
}

// TestRoute_QuietHoursOnlyAlertAndActBreakThrough pins the headline §6.3
// guarantee on its own: during quiet hours, NOTHING below alert/act is ever
// delivered immediately — every dial, every lower disposition is held or
// dropped, never DeliverNow. (The matrix test covers the values; this asserts
// the invariant directly so its intent survives a table edit.)
func TestRoute_QuietHoursOnlyAlertAndActBreakThrough(t *testing.T) {
	dials := []Dial{DialQuiet, DialBalanced, DialChatty}
	for _, dial := range dials {
		// alert + act MUST break through quiet hours.
		for _, d := range []Disposition{DispAlert, DispAct} {
			if got := Route(dial, d, true); got != DeliverNow {
				t.Errorf("quiet hours: Route(%s, %s) = %s, want DeliverNow (must break through)", dial, d, got)
			}
		}
		// everything below alert/act MUST NOT deliver now during quiet hours.
		for _, d := range []Disposition{DispNotify, DispDigest, DispDrop} {
			if got := Route(dial, d, true); got == DeliverNow {
				t.Errorf("quiet hours: Route(%s, %s) = DeliverNow — only alert/act may break quiet hours", dial, d)
			}
		}
	}
}
