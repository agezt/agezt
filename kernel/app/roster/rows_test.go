// SPDX-License-Identifier: MIT

package roster

// The roster's request rows live in the contract (kernel/contract/opapi), and
// the family's own rules live in the family: a ref is required and is not
// trimmed, a page size that is not a page size is the family's default, and
// the graveyard's age is a number — or the string a query string carries for
// one.
//
// The rows are read through the daemon's door, so a mistyped field is refused
// by the door with its own wording, before any effect — and a field that was
// not sent is not the same answer as a field that was sent as zero.

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/modules/agents/roster"
)

// rosterAuth admits the operator the roster's read operations are served to.
type rosterAuth struct{ principal opapi.Principal }

func (a rosterAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return a.principal, nil
}

// rosterRoute routes every operation to its caller.
type rosterRoute struct{}

func (rosterRoute) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

// TestTheRosterRowsCarryTheFamiliesOwnRules says the page size keeps the
// family's rules, that the ref is required and is not trimmed, and that the
// graveyard's age is a number — or the string that carries one.
func TestTheRosterRowsCarryTheFamiliesOwnRules(t *testing.T) {
	events := []*event.Event{}
	for i := int64(1); i <= 60; i++ {
		events = append(events, activityEvent(i, event.KindTaskReceived, "run-"+strconv.FormatInt(i, 10), map[string]any{"agent": "a"}))
	}
	service, reads := activityFixture(events)
	ops, err := ActivityOperations(func(context.Context) *ActivityService { return service })
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: rosterAuth{opapi.Principal{Kind: opapi.Operator}}, Router: rosterRoute{}})
	if err != nil {
		t.Fatal(err)
	}
	// The page size: the family's default when nothing was asked for, the
	// caller's count when one was, and the family's cap when the caller asked
	// for too many.
	for _, tc := range []struct {
		raw   string
		count int
	}{
		{`{"ref":"a"}`, 50},
		{`{"ref":"a","limit":3}`, 3},
		{`{"ref":"a","limit":1.9}`, 1},
		{`{"ref":"a","limit":0}`, 50},
		{`{"ref":"a","limit":-1}`, 50},
		{`{"ref":"a","limit":0.5}`, 60},
		{`{"ref":"a","limit":9999}`, 60},
	} {
		out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_activity", json.RawMessage(tc.raw), nil)
		if err != nil {
			t.Fatalf("activity %s: %v", tc.raw, err)
		}
		page, ok := out.(ActivityOutput)
		if !ok || page.Count != tc.count {
			t.Fatalf("activity %s count=%d, want %d", tc.raw, page.Count, tc.count)
		}
	}
	// The ref is required, and it is not trimmed: the agent's name is the name
	// it was given.
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_activity", json.RawMessage(`{}`), nil)
	if err == nil || err.Error() != "args.ref required" || out != nil {
		t.Fatalf("a missing ref: %v", err)
	}
	out, err = d.Dispatch(context.Background(), opapi.Caller{}, "agent_activity", json.RawMessage(`{"ref":" a "}`), nil)
	if err == nil || err.Error() != "unknown agent:  a " || out != nil {
		t.Fatalf("an untrimmed ref: %v", err)
	}
	// A mistyped field is a usage error the door names — before any effect.
	audited := *reads
	for _, raw := range []string{`{"ref":7}`, `{"ref":"a","limit":"1"}`, `{"ref":"a","cursor":7}`} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_activity", json.RawMessage(raw), nil); err == nil {
			t.Fatalf("a mistyped field was accepted %s", raw)
		} else if !strings.Contains(err.Error(), "has wrong type") {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	if *reads != audited {
		t.Fatalf("a refused body was read %d times", *reads-audited)
	}
	// The graveyard's age is a number — or the string a query string carries
	// for one. The two are the same answer.
	now := time.UnixMilli(100 * graveyardDay)
	profiles := []core.Profile{
		{Slug: "late", Name: "Late", Retired: true, RetiredMS: now.UnixMilli() - graveyardDay/2},
		{Slug: "old", Name: "Old", Retired: true, RetiredMS: now.UnixMilli() - 10*graveyardDay - 1},
	}
	graveyard, err := GraveyardOperations(func(context.Context) *GraveyardService {
		return NewGraveyard(func() []core.Profile { return profiles }, func() time.Time { return now })
	})
	if err != nil {
		t.Fatal(err)
	}
	gd, err := app.NewDispatcher(graveyard, app.Dependencies{Auth: rosterAuth{opapi.Principal{Kind: opapi.Operator}}, Router: rosterRoute{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"older_than_days":5}`, `{"older_than_days":"5"}`, `{"older_than_days":" 5 "}`} {
		out, err := gd.Dispatch(context.Background(), opapi.Caller{}, "agent_graveyard", json.RawMessage(raw), nil)
		if err != nil {
			t.Fatalf("graveyard %s: %v", raw, err)
		}
		report, ok := out.(GraveyardOutput)
		if !ok || report.Count != 1 || report.Graveyard[0].Slug != "old" || report.OlderThanDays != 5 {
			t.Fatalf("graveyard %s report=%+v", raw, report)
		}
	}
	if _, err := gd.Dispatch(context.Background(), opapi.Caller{}, "agent_graveyard", json.RawMessage(`{"older_than_days":true}`), nil); err == nil {
		t.Fatal("a mistyped age was accepted")
	} else if !strings.Contains(err.Error(), "has wrong type") {
		t.Fatalf("a mistyped age: %v", err)
	}
}
