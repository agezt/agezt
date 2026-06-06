// SPDX-License-Identifier: MIT

package webhook

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

// TestProbeOK_StatusBoundary pins the 2xx success window of ProbeResult.OK across
// both edges. The existing probe tests only cover 200 (OK) and 500 (not OK), so the
// upper bound `status < 300` was unpinned: mutation testing (M510) showed `< → <=`
// survived because no case sits on 300 — under the mutant a 300 ("Multiple Choices",
// which Go does NOT auto-follow) would be wrongly reported as a successful delivery.
func TestProbeOK_StatusBoundary(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{199, false}, // just below the window
		{200, true},  // lower edge, inclusive
		{201, true},
		{299, true},  // upper edge, inclusive
		{300, false}, // first code outside the window — must NOT count as delivered
		{400, false},
		{500, false},
	}
	for _, c := range cases {
		got := ProbeResult{Status: c.status}.OK()
		if got != c.want {
			t.Errorf("OK(status=%d) = %v, want %v", c.status, got, c.want)
		}
	}
}

// TestDispatch_Status300IsFailure pins the same success-window upper edge on the live
// delivery path (deliver's `status >= 200 && status < 300`). A sink returning 300 must
// be treated as a failed delivery — retried and ultimately journaled webhook.failed,
// never webhook.delivered. The existing dispatch tests cover 200 (delivered) and 500
// (failed) but never 300, leaving `< 300 → <= 300` alive on this copy of the boundary.
func TestDispatch_Status300IsFailure(t *testing.T) {
	cap := &capture{statuses: []int{300, 300, 300}}
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	b := newBus(t)
	audit := newAuditSink(t, b)
	d := NewDispatcher(b, []Sink{{URL: srv.URL, Subject: ">"}}, nil)
	d.MaxAttempts = 3
	d.Backoff = func(int) time.Duration { return 0 }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	_, _ = b.Publish(event.Spec{Subject: "agent.x.task", Kind: event.KindTaskCompleted, Actor: "a"})
	// 300 is not 2xx, so every attempt is treated as a failure → all MaxAttempts are spent.
	waitFor(t, func() bool { return cap.count() == 3 })
	waitFor(t, func() bool { return audit.has(event.KindWebhookFailed) })
	if audit.has(event.KindWebhookDelivered) {
		t.Error("status 300 must not be journaled as delivered")
	}
}
