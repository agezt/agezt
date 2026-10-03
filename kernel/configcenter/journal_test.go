// SPDX-License-Identifier: MIT

package configcenter

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// TestAccessIsJournaled: agents read secrets through the config center, and its
// audit trail lived only in this package's files, outside the hash-chained
// journal (config.access was declared but never emitted). Every access is now a
// config.access event carrying the decision — and never the value, because the
// journal cannot be purged.
func TestAccessIsJournaled(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(filepath.Join(dir, "journal"), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	c, err := New(DefaultConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	c.SetBus(bus.New(j))
	const value = "visible-public-value"
	if err := c.Set(&ConfigEntry{Key: "feature.flag", Value: value, Rating: RatingPublic}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), ConfigAccessRequest{AgentID: "agent-7", RunID: "run-1", Key: "feature.flag"}); err != nil {
		t.Fatal(err)
	}

	var got []*event.Event
	if err := j.Range(func(e *event.Event) error {
		if e.Kind == event.KindConfigAccess {
			got = append(got, e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("config.access events = %d, want 1", len(got))
	}
	e := got[0]
	var p map[string]any
	_ = json.Unmarshal(e.Payload, &p)
	if e.Actor != "agent-7" || e.CorrelationID != "run-1" || p["key"] != "feature.flag" || p["decision"] != string(AccessAllowed) {
		t.Fatalf("event = actor %q corr %q payload %v", e.Actor, e.CorrelationID, p)
	}
	if strings.Contains(string(e.Payload), value) {
		t.Fatalf("the journal (no purge path) received the value: %s", e.Payload)
	}
}
