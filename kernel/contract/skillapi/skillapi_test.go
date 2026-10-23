// SPDX-License-Identifier: MIT

package skillapi

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// wireKeys returns the JSON object keys of the marshalled value, sorted.
func wireKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := make([]string, 0, len(wire))
	for k := range wire {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestTheStatusValuesAreThePersistedOnes: the lifecycle states are persisted
// on disk and read back by the CLI and the Web UI, so their VALUES are a
// contract — a renamed state would orphan every stored skill.
func TestTheStatusValuesAreThePersistedOnes(t *testing.T) {
	for _, c := range []struct {
		status Status
		want   string
	}{
		{StatusDraft, "draft"},
		{StatusShadow, "shadow"},
		{StatusActive, "active"},
		{StatusQuarantined, "quarantined"},
		{StatusArchived, "archived"},
	} {
		if string(c.status) != c.want {
			t.Errorf("the status value = %q, want %q", string(c.status), c.want)
		}
	}
	if DefaultVersion != "0.1.0" {
		t.Errorf("the default version = %q, want 0.1.0", DefaultVersion)
	}
}

// TestTheLifecycleRulesTravelWithTheRow: the lifecycle rules are pure, so
// they belong where the row is — and they are the rule the whole daemon
// enforces, so every edge is measured.
func TestTheLifecycleRulesTravelWithTheRow(t *testing.T) {
	legal := [][2]Status{
		{StatusDraft, StatusShadow},
		{StatusDraft, StatusArchived},
		{StatusShadow, StatusActive},
		{StatusShadow, StatusQuarantined},
		{StatusShadow, StatusArchived},
		{StatusActive, StatusQuarantined},
		{StatusActive, StatusArchived},
		{StatusQuarantined, StatusActive},
		{StatusQuarantined, StatusArchived},
	}
	for _, e := range legal {
		if !CanTransition(e[0], e[1]) {
			t.Errorf("the transition %s → %s must be legal", e[0], e[1])
		}
	}
	illegal := [][2]Status{
		{StatusDraft, StatusActive},
		{StatusShadow, StatusDraft},
		{StatusActive, StatusShadow},
		{StatusActive, StatusDraft},
		{StatusArchived, StatusActive},
		{StatusArchived, StatusDraft},
	}
	for _, e := range illegal {
		if CanTransition(e[0], e[1]) {
			t.Errorf("the transition %s → %s must be illegal", e[0], e[1])
		}
	}
	// An unknown state is never a legal source.
	if CanTransition("bogus", StatusActive) {
		t.Error("an unknown status must have no outgoing transition")
	}
}

// TestValidStatusCoversExactlyThePersistedStates: the persisted states are
// the five; anything else would be written and never read back.
func TestValidStatusCoversExactlyThePersistedStates(t *testing.T) {
	for _, s := range []Status{StatusDraft, StatusShadow, StatusActive, StatusQuarantined, StatusArchived} {
		if !ValidStatus(s) {
			t.Errorf("the status %q must be valid", s)
		}
	}
	for _, s := range []Status{"", "bogus", "DRAFT", "retired"} {
		if ValidStatus(s) {
			t.Errorf("the status %q must be invalid", s)
		}
	}
}

// TestPromoteTargetWalksTheLadder: promote advances draft→shadow→active and
// un-quarantines back into production — and it is defined nowhere else.
func TestPromoteTargetWalksTheLadder(t *testing.T) {
	for _, c := range []struct {
		from Status
		to   Status
		ok   bool
	}{
		{StatusDraft, StatusShadow, true},
		{StatusShadow, StatusActive, true},
		{StatusQuarantined, StatusActive, true},
		{StatusActive, StatusActive, false},
		{StatusArchived, StatusArchived, false},
	} {
		got, ok := PromoteTarget(c.from)
		if ok != c.ok || (ok && got != c.to) {
			t.Errorf("PromoteTarget(%s) = %s, %v; want %s, %v", c.from, got, ok, c.to, c.ok)
		}
	}
}

// TestTheSkillRowWireShape: the skill record is persisted as skills.json and
// read back by key, so its JSON keys are a contract with every file already on
// disk. A renamed key would silently drop that field.
func TestTheSkillRowWireShape(t *testing.T) {
	got := wireKeys(t, Skill{})
	want := []string{
		"body", "created_ms", "description", "id", "last_seen_ms", "metrics",
		"name", "status", "version",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the skill wire keys = %v, want %v", got, want)
	}
}

// TestTheSkillAlwaysFieldsStayOnTheWire: the fields that describe the record
// are always present — a missing "status" would read as the empty string, and
// a missing "body" as a skill with no procedure.
func TestTheSkillAlwaysFieldsStayOnTheWire(t *testing.T) {
	raw, err := json.Marshal(Skill{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{
		"id", "name", "description", "body", "version", "status", "metrics",
		"created_ms", "last_seen_ms",
	} {
		if _, ok := wire[k]; !ok {
			t.Errorf("the skill wire is missing the always-field %q: %s", k, raw)
		}
	}
	// The optional ones stay off the wire when empty.
	for _, k := range []string{"triggers", "tools_required", "resources", "agent", "lineage", "source_event"} {
		if _, ok := wire[k]; ok {
			t.Errorf("an empty %q must stay off the wire: %s", k, raw)
		}
	}
}

// TestTheMetricsRowWireShape: the metrics travel inside the skill record, so
// their keys are pinned too.
func TestTheMetricsRowWireShape(t *testing.T) {
	got := wireKeys(t, Metrics{})
	want := []string{"failures", "successes", "uses"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the metrics wire keys = %v, want %v", got, want)
	}
}

// TestActiveIsTheRetrievalPoolMembership: the one predicate on the record.
func TestActiveIsTheRetrievalPoolMembership(t *testing.T) {
	if !(Skill{Status: StatusActive}).Active() {
		t.Error("an active skill is in the retrieval pool")
	}
	for _, s := range []Status{StatusDraft, StatusShadow, StatusQuarantined, StatusArchived} {
		if (Skill{Status: s}).Active() {
			t.Errorf("a %s skill is not in the retrieval pool", s)
		}
	}
}

// TestThePortShapes: the two ports are the whole surface the agent-facing
// tool drives. A port that grows a method grows the tool's reach.
func TestThePortShapes(t *testing.T) {
	forge := reflect.TypeOf((*Forge)(nil)).Elem()
	wantForge := []string{"Create", "Get", "List", "Promote", "Quarantine"}
	gotForge := make([]string, 0, forge.NumMethod())
	for i := 0; i < forge.NumMethod(); i++ {
		gotForge = append(gotForge, forge.Method(i).Name)
	}
	sort.Strings(gotForge)
	if !reflect.DeepEqual(gotForge, wantForge) {
		t.Errorf("the forge port methods = %v, want %v", gotForge, wantForge)
	}

	bundles := reflect.TypeOf((*BundleStore)(nil)).Elem()
	wantBundles := []string{"Dir", "List", "Read"}
	gotBundles := make([]string, 0, bundles.NumMethod())
	for i := 0; i < bundles.NumMethod(); i++ {
		gotBundles = append(gotBundles, bundles.Method(i).Name)
	}
	sort.Strings(gotBundles)
	if !reflect.DeepEqual(gotBundles, wantBundles) {
		t.Errorf("the bundle port methods = %v, want %v", gotBundles, wantBundles)
	}
}
