// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

const graveyardDay = 24 * 3600 * 1000

func TestRosterGraveyardCodecMatchesLegacyLeniency(t *testing.T) {
	for raw, want := range map[string]float64{
		`{}`: 0, `{"older_than_days":3}`: 3, `{"older_than_days":2.75}`: 2.75, `{"older_than_days":" 4.5 "}`: 4.5,
		`{"older_than_days":"soon"}`: 0, `{"older_than_days":true}`: 0, `{"older_than_days":null}`: 0, `{"older_than_days":[1]}`: 0,
		`{"older_than_days":-2}`: -2, `{"older_than_days":"1e400"}`: math.Inf(1),
	} {
		var in GraveyardRequest
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			t.Fatal(raw, err)
		}
		if got := in.days(); got != want {
			t.Fatal(raw, got, want)
		}
	}
}

func TestRosterGraveyardFiltersOrdersAndAges(t *testing.T) {
	now := time.UnixMilli(100 * graveyardDay)
	managed := false
	profiles := []core.Profile{
		{Slug: "live", Name: "Live", Enabled: true},
		{Slug: "late", Name: "Late", Retired: true, RetiredMS: now.UnixMilli() - graveyardDay/2, RetiredReason: "recent"},
		{Slug: "old", Name: "Old", Retired: true, RetiredMS: now.UnixMilli() - 10*graveyardDay - 1, System: true},
		{Slug: "tie-b", Retired: true, RetiredMS: now.UnixMilli() - 5*graveyardDay, DirectCallable: &managed},
		{Slug: "tie-a", Retired: true, RetiredMS: now.UnixMilli() - 5*graveyardDay},
		{Slug: "unknown", Retired: true},
	}
	s := NewGraveyard(func() []core.Profile { return profiles }, func() time.Time { return now })
	all := s.Graveyard(context.Background(), 0)
	slugs := func(out GraveyardOutput) (got []string) {
		for _, row := range out.Graveyard {
			got = append(got, row.Slug)
		}
		return got
	}
	if !reflect.DeepEqual(slugs(all), []string{"unknown", "old", "tie-b", "tie-a", "late"}) || all.Count != 5 || all.OlderThanDays != 0 {
		t.Fatal("order", slugs(all), all)
	}
	want := map[string]GraveyardRow{
		"unknown": {Slug: "unknown", Kind: "custom"},
		"old":     {Slug: "old", Name: "Old", Kind: "system", System: true, RetiredMS: now.UnixMilli() - 10*graveyardDay - 1, AgeDays: 10},
		"tie-b":   {Slug: "tie-b", Kind: "subagent", RetiredMS: now.UnixMilli() - 5*graveyardDay, AgeDays: 5},
		"late":    {Slug: "late", Name: "Late", Kind: "custom", RetiredMS: now.UnixMilli() - graveyardDay/2, RetiredReason: "recent"},
	}
	for _, row := range all.Graveyard {
		if w, ok := want[row.Slug]; ok && row != w {
			t.Fatal(row, w)
		}
	}
	for days, label := range map[float64]int{5: 5, 4.9: 4} {
		filtered := s.Graveyard(context.Background(), days)
		if !reflect.DeepEqual(slugs(filtered), []string{"unknown", "old", "tie-b", "tie-a"}) || filtered.OlderThanDays != label {
			t.Fatal("cutoff keeps exact boundary and unknown age", days, slugs(filtered), filtered.OlderThanDays)
		}
	}
	if out := s.Graveyard(context.Background(), 5.01); !reflect.DeepEqual(slugs(out), []string{"unknown", "old"}) {
		t.Fatal("cutoff just past boundary", slugs(out))
	}
	if out := s.Graveyard(context.Background(), 10); !reflect.DeepEqual(slugs(out), []string{"unknown", "old"}) {
		t.Fatal("cutoff", slugs(out))
	}
	if out := s.Graveyard(context.Background(), 0.75); out.Count != 4 || out.OlderThanDays != 0 || slugs(out)[3] != "tie-a" {
		t.Fatal("sub-day window filters", slugs(out), out.OlderThanDays)
	}
	if out := s.Graveyard(context.Background(), -3); out.Count != 5 || out.OlderThanDays != -3 {
		t.Fatal("negative window is no filter", out)
	}
	raw, _ := json.Marshal(NewGraveyard(func() []core.Profile { return profiles[:1] }, nil).Graveyard(context.Background(), 0))
	if string(raw) != `{"graveyard":[],"count":0,"older_than_days":0}` {
		t.Fatal("empty graveyard shape", string(raw))
	}
}

func TestRosterGraveyardOperationSpecAndAdmission(t *testing.T) {
	if _, err := GraveyardOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	calls := 0
	profiles := []core.Profile{{Slug: "gone", Retired: true, RetiredMS: 1}}
	ops, err := GraveyardOperations(func(ctx context.Context) *GraveyardService {
		calls++
		if ctx.Value(listRouteKey{}) != "selected" {
			t.Fatal("route lost")
		}
		return NewGraveyard(func() []core.Profile { return profiles }, func() time.Time { return time.UnixMilli(3 * graveyardDay) })
	})
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	s := ops[0].Spec()
	if s.Name != "agent_graveyard" || !s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || !s.AllowUnknownInput || s.Input != reflect.TypeFor[GraveyardRequest]() || s.Output != reflect.TypeFor[GraveyardOutput]() || s.Stream != opapi.StreamNone || s.HTTP != (opapi.HTTP{}) {
		t.Fatal(s)
	}
	if schema.ValidateJSON(s.OutputSchema, json.RawMessage(`{"graveyard":[{"age_days":"1","kind":"","name":"","retired_ms":0,"retired_reason":"","slug":"","system":false}],"count":1,"older_than_days":0}`)) == nil {
		t.Fatal("untyped row accepted")
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}})
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_graveyard", json.RawMessage(`{}`), nil); err == nil || calls != 0 {
			t.Fatal("non-primary effects", err)
		}
	}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_graveyard", json.RawMessage(`{}`), nil); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled effects", err)
	}
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_graveyard", json.RawMessage(`{"older_than_days":"2","unknown":true}`), nil)
	page, ok := out.(GraveyardOutput)
	if err != nil || !ok || page.Count != 1 || page.OlderThanDays != 2 || page.Graveyard[0].AgeDays != 2 || calls != 1 {
		t.Fatal(out, err)
	}
}

func TestRosterGraveyardKeepsRosterOrderWithinTies(t *testing.T) {
	var profiles []core.Profile
	for i := range 64 {
		profiles = append(profiles, core.Profile{Slug: strconv.Itoa(i), Retired: true, RetiredMS: int64(1 + i%2)})
	}
	out := NewGraveyard(func() []core.Profile { return profiles }, func() time.Time { return time.UnixMilli(graveyardDay) }).Graveyard(context.Background(), 0)
	last := map[int64]int{1: -1, 2: -1}
	for i, row := range out.Graveyard {
		n, _ := strconv.Atoi(row.Slug)
		if (i < 32) != (row.RetiredMS == 1) || n <= last[row.RetiredMS] {
			t.Fatal("stable oldest-first order", i, row)
		}
		last[row.RetiredMS] = n
	}
}
