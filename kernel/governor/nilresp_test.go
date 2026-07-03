// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/governor"
)

// TestComplete_NilResponse_ReturnsError: a provider that violates the contract
// by returning (nil, nil) must be normalized into an error at the governor —
// the one choke point every governed model call flows through — so the ~20
// downstream resp.Message derefs (research/council/conductor/verify/…) degrade
// via their existing err checks instead of panicking the daemon.
func TestComplete_NilResponse_ReturnsError(t *testing.T) {
	b, _ := newBus(t)
	prov := &fakeProvider{name: "nilprov"} // resp=nil, err=nil → returns (nil, nil)
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{Name: "nilprov", Provider: prov, AuthMode: governor.AuthLocal})
	g, err := governor.New(governor.Config{Registry: r, Bus: b})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "m",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "q"}},
	})
	if err == nil {
		t.Fatal("Complete returned a nil error for a (nil,nil) provider response; want an error, not a downstream panic")
	}
	if !strings.Contains(err.Error(), "nil response") {
		t.Errorf("err = %v; want a nil-response error", err)
	}
	if resp != nil {
		t.Errorf("resp = %+v; want nil", resp)
	}
}
