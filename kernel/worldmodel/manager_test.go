// SPDX-License-Identifier: MIT

package worldmodel

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// entityRMWProbe tracks the maximum number of overlapping GetEntity→PutEntity
// windows (concurrent reinforces). With the Graph mutex held across Upsert's
// Get→Put, the max stays 1; without it, concurrent writers overlap (M421).
type entityRMWProbe struct {
	Store
	mu       sync.Mutex
	inFlight int
	maxConc  int
}

func (p *entityRMWProbe) GetEntity(id string) (Entity, bool, error) {
	p.mu.Lock()
	p.inFlight++
	if p.inFlight > p.maxConc {
		p.maxConc = p.inFlight
	}
	p.mu.Unlock()
	time.Sleep(2 * time.Millisecond) // widen the RMW window
	return p.Store.GetEntity(id)
}

func (p *entityRMWProbe) PutEntity(e Entity) error {
	err := p.Store.PutEntity(e)
	p.mu.Lock()
	p.inFlight--
	p.mu.Unlock()
	return err
}

func (p *entityRMWProbe) maxConcurrent() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maxConc
}

// TestGraph_SerializesConcurrentUpserts: the Graph must hold a lock across Upsert's
// GetEntity→PutEntity so concurrent reinforces (or a reinforce racing Decay) can't
// interleave and lose an update (M421). Verified structurally — no two RMW windows
// overlap with the lock held.
func TestGraph_SerializesConcurrentUpserts(t *testing.T) {
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	base, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("worldmodel.Open: %v", err)
	}
	probe := &entityRMWProbe{Store: base}
	g := NewGraph(probe, b)

	const N = 8
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = g.Upsert("c", UpsertSpec{Kind: KindProject, Name: "Portfolio"})
		}()
	}
	wg.Wait()

	if got := probe.maxConcurrent(); got != 1 {
		t.Errorf("overlapping read-modify-write windows (maxConcurrent=%d, want 1): the Graph lock must serialize writes", got)
	}
}

var fixedNow = time.Unix(1_700_000_000, 0).UTC()

func newTestGraph(t *testing.T) (*Graph, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("worldmodel.Open: %v", err)
	}
	g := NewGraph(s, b)
	g.now = func() time.Time { return fixedNow }
	t.Cleanup(func() { b.Close(); j.Close() })
	return g, j
}

func countKind(t *testing.T, j *journal.Journal, k event.Kind) int {
	t.Helper()
	n := 0
	if err := j.Range(func(e *event.Event) error {
		if e.Kind == k {
			n++
		}
		return nil
	}); err != nil {
		t.Fatalf("range: %v", err)
	}
	return n
}

func TestUpsertCreatesAndJournals(t *testing.T) {
	g, j := newTestGraph(t)
	e, created, err := g.Upsert("corr-1", UpsertSpec{Kind: KindProject, Name: "Lictor", Aliases: []string{"portfolio"}})
	if err != nil || !created {
		t.Fatalf("upsert: created=%v err=%v", created, err)
	}
	if e.SourceEvent == "" {
		t.Fatal("created entity must carry provenance (source_event)")
	}
	if countKind(t, j, event.KindWorldEntityUpserted) != 1 {
		t.Errorf("expected 1 entity.upserted event")
	}
}

// TestUpsertDoesNotResurrectSuperseded: reinforcing an entity that was superseded
// must keep it inactive — the supersession link must survive the reinforce (M420).
func TestUpsertDoesNotResurrectSuperseded(t *testing.T) {
	g, _ := newTestGraph(t)
	e, _, err := g.Upsert("c", UpsertSpec{Kind: KindProject, Name: "Portfolio"})
	if err != nil {
		t.Fatal(err)
	}
	// Mark it superseded directly (no public graph-level Supersede).
	e.SupersededBy = "successor-id"
	if err := g.store.PutEntity(e); err != nil {
		t.Fatal(err)
	}
	// Reinforce by upserting the same name again.
	if _, _, err := g.Upsert("c", UpsertSpec{Kind: KindProject, Name: "Portfolio"}); err != nil {
		t.Fatal(err)
	}
	got, found, _ := g.store.GetEntity(e.ID)
	if !found {
		t.Fatal("entity vanished")
	}
	if got.SupersededBy != "successor-id" {
		t.Fatalf("reinforce cleared SupersededBy: %q, want successor-id", got.SupersededBy)
	}
	if got.Active() {
		t.Fatal("a superseded entity must not become active after reinforce")
	}
}

func TestUpsertReinforcesAndMergesAliases(t *testing.T) {
	g, _ := newTestGraph(t)
	first, _, _ := g.Upsert("c", UpsertSpec{Kind: KindProject, Name: "Lictor", Aliases: []string{"portfolio"}, Weight: 0.5})
	second, created, err := g.Upsert("c", UpsertSpec{Kind: KindProject, Name: "Lictor", Aliases: []string{"the repos"}})
	if err != nil {
		t.Fatalf("reinforce: %v", err)
	}
	if created {
		t.Error("same (kind,name) must dedupe, not create")
	}
	if second.ID != first.ID {
		t.Error("content-address should match")
	}
	if second.Weight <= first.Weight {
		t.Errorf("weight should strengthen: %v -> %v", first.Weight, second.Weight)
	}
	if len(second.Aliases) != 2 {
		t.Errorf("aliases should merge to 2, got %v", second.Aliases)
	}
}

func TestForgetRevive(t *testing.T) {
	g, _ := newTestGraph(t)
	e, _, _ := g.Upsert("c", UpsertSpec{Kind: KindProject, Name: "Lictor"})
	ok, err := g.Forget("c", e.ID)
	if err != nil || !ok {
		t.Fatalf("forget: ok=%v err=%v", ok, err)
	}
	if ents, _ := g.Entities(); len(ents) != 0 {
		t.Errorf("forgotten entity should not be active, got %d", len(ents))
	}
	// Re-add revives.
	revived, created, _ := g.Upsert("c", UpsertSpec{Kind: KindProject, Name: "Lictor"})
	if created {
		t.Error("revive should not report created")
	}
	if revived.Tombstoned {
		t.Error("revived entity should be active")
	}
}

func TestRelateResolvesEndpointsAndJournals(t *testing.T) {
	g, j := newTestGraph(t)
	_, _, _ = g.Upsert("c", UpsertSpec{Kind: KindProject, Name: "Lictor"})
	r, err := g.Relate("c", "Lictor", VerbDependsOn, "go-stdlib")
	if err != nil {
		t.Fatalf("relate: %v", err)
	}
	if r.Verb != VerbDependsOn {
		t.Errorf("verb = %s", r.Verb)
	}
	// go-stdlib was auto-created as a topic, so 2 entities now.
	if c := g.Count(); c != 2 {
		t.Errorf("expected 2 entities (Lictor + auto go-stdlib), got %d", c)
	}
	if countKind(t, j, event.KindWorldRelationUpserted) != 1 {
		t.Errorf("expected 1 relation.upserted event")
	}
	ns, _ := g.Neighbors(r.From)
	if len(ns) != 1 || ns[0].Other.Name != "go-stdlib" {
		t.Errorf("neighbors wrong: %+v", ns)
	}
}

func TestRelateDeduplicatesAndReinforces(t *testing.T) {
	g, j := newTestGraph(t)
	_, _, _ = g.Upsert("c", UpsertSpec{Kind: KindProject, Name: "Lictor"})

	first, err := g.Relate("c", "Lictor", VerbDependsOn, "go-stdlib")
	if err != nil {
		t.Fatalf("first relate: %v", err)
	}
	second, err := g.Relate("c", "Lictor", VerbDependsOn, "go-stdlib")
	if err != nil {
		t.Fatalf("second relate: %v", err)
	}

	// Same (from, verb, to) → same RelationID → ONE edge, not two. The graph must
	// not accumulate duplicate relations when the model restates a known link.
	if first.ID != second.ID {
		t.Errorf("duplicate relate produced different IDs: %s vs %s", first.ID, second.ID)
	}
	rels, _ := g.Relations()
	if len(rels) != 1 {
		t.Fatalf("expected exactly 1 relation after a duplicate relate, got %d", len(rels))
	}
	// Weight is already at the clamp ceiling (1.0) from creation; reinforcing keeps
	// it there rather than overflowing past the cap.
	if second.Weight != 1.0 {
		t.Errorf("reinforced weight = %v, want 1.0 (clamped)", second.Weight)
	}
	// CreatedMS is preserved across the reinforce; the second is a reinforce event.
	if second.CreatedMS != first.CreatedMS {
		t.Errorf("reinforce must preserve CreatedMS: first=%d second=%d", first.CreatedMS, second.CreatedMS)
	}
	if n := countKind(t, j, event.KindWorldRelationUpserted); n != 2 {
		t.Errorf("expected 2 relation.upserted events (create + reinforce), got %d", n)
	}
}

func TestRelateRejectsEmptyName(t *testing.T) {
	g, _ := newTestGraph(t)
	if _, err := g.Relate("c", "", VerbDependsOn, "x"); err != ErrEmptyName {
		t.Errorf("empty from-name: err=%v, want ErrEmptyName", err)
	}
	if _, err := g.Relate("c", "x", VerbDependsOn, "   "); err != ErrEmptyName {
		t.Errorf("blank to-name: err=%v, want ErrEmptyName", err)
	}
}

func TestResolveJournalsRetrieved(t *testing.T) {
	g, j := newTestGraph(t)
	_, _, _ = g.Upsert("c", UpsertSpec{Kind: KindProject, Name: "Lictor", Aliases: []string{"the portfolio"}})
	hits, err := g.Resolve("run-1", "the portfolio", 5)
	if err != nil || len(hits) == 0 {
		t.Fatalf("resolve: hits=%d err=%v", len(hits), err)
	}
	if countKind(t, j, event.KindWorldRetrieved) != 1 {
		t.Errorf("resolve with hits should journal worldmodel.retrieved")
	}
	// A miss journals nothing.
	_, _ = g.Resolve("run-1", "nonexistent-zzz", 5)
	if countKind(t, j, event.KindWorldRetrieved) != 1 {
		t.Errorf("resolve miss should not journal")
	}
}

func TestIsActiveSubject(t *testing.T) {
	g, _ := newTestGraph(t)
	_, _, _ = g.Upsert("c", UpsertSpec{Kind: KindProject, Name: "Lictor", Aliases: []string{"portfolio"}})
	if name, ok := g.IsActiveSubject("the portfolio is broken"); !ok || name != "Lictor" {
		t.Errorf("known subject should match: name=%q ok=%v", name, ok)
	}
	if _, ok := g.IsActiveSubject("completely unrelated thing"); ok {
		t.Error("unknown subject should not match")
	}
}

func TestNilBusStoreOnly(t *testing.T) {
	s, _ := Open(t.TempDir())
	g := NewGraph(s, nil) // no bus — store-only
	g.now = func() time.Time { return fixedNow }
	if _, _, err := g.Upsert("", UpsertSpec{Kind: KindProject, Name: "X"}); err != nil {
		t.Fatalf("upsert without bus should work: %v", err)
	}
}

func TestWorldToolRoundTrip(t *testing.T) {
	g, _ := newTestGraph(t)
	tool := g.Tool()
	if tool.Definition().Name != "world" {
		t.Fatalf("tool name = %q", tool.Definition().Name)
	}
	ctx := WithCorrelation(context.Background(), "tool-corr")

	add, _ := json.Marshal(map[string]any{"action": "add", "kind": "project", "name": "Lictor", "aliases": []string{"portfolio"}})
	if res, _ := tool.Invoke(ctx, add); res.IsError {
		t.Fatalf("add failed: %s", res.Output)
	}
	rel, _ := json.Marshal(map[string]any{"action": "relate", "from": "Lictor", "verb": "depends_on", "to": "go-stdlib"})
	if res, _ := tool.Invoke(ctx, rel); res.IsError {
		t.Fatalf("relate failed: %s", res.Output)
	}
	res, _ := tool.Invoke(ctx, mustJSON(t, map[string]any{"action": "resolve", "query": "the portfolio"}))
	if res.IsError || !strings.Contains(res.Output, "Lictor") {
		t.Fatalf("resolve output = %q", res.Output)
	}
	res, _ = tool.Invoke(ctx, mustJSON(t, map[string]any{"action": "neighbors", "query": "Lictor"}))
	if res.IsError || !strings.Contains(res.Output, "go-stdlib") {
		t.Fatalf("neighbors output = %q", res.Output)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

var _ = agent.Tool(worldTool{})
