// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

func ev(kind event.Kind, payload map[string]any) *event.Event {
	b, _ := json.Marshal(payload)
	return &event.Event{Kind: kind, Payload: b}
}

func TestAutonomyDetail(t *testing.T) {
	cases := []struct {
		name string
		e    *event.Event
		want string
	}{
		{"schedule intent", ev(event.KindScheduleFired, map[string]any{"intent": "digest the inbox"}), "digest the inbox"},
		{"standing name", ev(event.KindStandingCreated, map[string]any{"name": "watch CI"}), "watch CI"},
		{"skill name", ev(event.KindSkillPromoted, map[string]any{"name": "diagnose-ci", "id": "abc"}), "diagnose-ci"},
		{"skill id fallback", ev(event.KindSkillCreated, map[string]any{"id": "deadbeef"}), "deadbeef"},
		{"assure complete", ev(event.KindAssureVerdict, map[string]any{"complete": true}), "complete: true"},
		{"assure gap", ev(event.KindAssureVerdict, map[string]any{"complete": false, "gap": "no tests"}), "gap: no tests"},
		{"briefing subject", ev(event.KindBriefingSent, map[string]any{"subject": "morning digest"}), "morning digest"},
		{"board topic+from", ev(event.KindBoardPosted, map[string]any{"topic": "acil-mudahale", "from": "watcher"}), "acil-mudahale · from watcher"},
		{"board topic only", ev(event.KindBoardPosted, map[string]any{"topic": "general"}), "general"},
		{"empty payload", &event.Event{Kind: event.KindScheduleFired}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := autonomyDetail(c.e); got != c.want {
				t.Errorf("autonomyDetail = %q, want %q", got, c.want)
			}
		})
	}
}

func TestAutonomyKinds_ExcludesReactiveNoise(t *testing.T) {
	// Reactive plumbing must NOT be in the curated feed.
	for _, k := range []event.Kind{event.KindLLMRequest, event.KindToolInvoked, event.KindTaskReceived} {
		if _, ok := autonomyKinds[k]; ok {
			t.Errorf("%q should be excluded from the autonomy feed", k)
		}
	}
	// Self-directed milestones must be present.
	for _, k := range []event.Kind{event.KindScheduleFired, event.KindSkillCreated, event.KindAssureVerdict} {
		if _, ok := autonomyKinds[k]; !ok {
			t.Errorf("%q should be in the autonomy feed", k)
		}
	}
}

func TestClipDetail(t *testing.T) {
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}
	if got := clipDetail(string(long)); len([]rune(got)) != 120 {
		t.Errorf("clipDetail length = %d, want 120", len([]rune(got)))
	}
	if clipDetail("short") != "short" {
		t.Error("short strings should pass through unchanged")
	}
}
