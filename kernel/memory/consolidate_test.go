// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

func consolidationCorpus() []Record {
	mk := func(id, subject, content, scope string) Record {
		r := Record{ID: id, Type: TypeFact, Subject: subject, Content: content, Confidence: 1, CreatedMS: 100, LastSeenMS: 100}
		if scope != "" {
			r.Tags = map[string]string{"scope": scope}
		}
		return r
	}
	return []Record{
		// A tight cluster: three near-duplicate kubernetes facts.
		mk("k1", "kubernetes", "the kubernetes cluster runs in frankfurt", ""),
		mk("k2", "kubernetes", "kubernetes cluster is hosted in frankfurt region", ""),
		mk("k3", "kubernetes", "our kubernetes cluster lives in frankfurt and needs weekly upgrades", ""),
		// A private-scope record about the same topic — must NOT join.
		mk("kp", "kubernetes", "kubernetes cluster frankfurt private operator note", "researcher"),
		// A lone unrelated record.
		mk("pz", "cooking", "pizza dough requires slow fermentation", ""),
	}
}

func TestClusters_GroupsNeighboursRespectsScopeAndMinSize(t *testing.T) {
	cs := Clusters(consolidationCorpus(), clusterCosine, minClusterSize)
	if len(cs) != 1 {
		t.Fatalf("clusters = %d, want 1 (got %+v)", len(cs), cs)
	}
	ids := map[string]bool{}
	for _, r := range cs[0] {
		ids[r.ID] = true
	}
	if !ids["k1"] || !ids["k2"] || !ids["k3"] {
		t.Fatalf("kubernetes cluster incomplete: %v", ids)
	}
	if ids["kp"] {
		t.Fatal("private-scope record leaked into the shared cluster")
	}
	if ids["pz"] {
		t.Fatal("unrelated record joined the cluster")
	}
	// Inactive records never cluster.
	rs := consolidationCorpus()
	for i := range rs {
		rs[i].Tombstoned = true
	}
	if got := Clusters(rs, clusterCosine, minClusterSize); len(got) != 0 {
		t.Fatalf("tombstoned records clustered: %d", len(got))
	}
}

// scriptedProvider answers Complete from a script, counting calls; once the
// script is exhausted it repeats the last answer.
type scriptedProvider struct {
	calls   int
	answers []string
}

func (p *scriptedProvider) Name() string { return "consolidation-mock" }

func (p *scriptedProvider) Complete(_ context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	i := p.calls
	if i >= len(p.answers) {
		i = len(p.answers) - 1
	}
	p.calls++
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: p.answers[i]},
		StopReason: agent.StopEndTurn,
	}, nil
}

// newConsolidationManager builds a bus-less Manager with a fixed clock.
func newConsolidationManager(t *testing.T) *Manager {
	t.Helper()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	m := NewManager(store, nil)
	m.now = func() time.Time { return time.UnixMilli(1000) }
	return m
}

func TestDistillBrain_MergesClusterAndSupersedes(t *testing.T) {
	m := newConsolidationManager(t)
	for _, r := range consolidationCorpus() {
		if _, _, err := m.Remember("", RememberSpec{Type: r.Type, Subject: r.Subject, Content: r.Content, Tags: r.Tags}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	prov := scriptedProvider{answers: []string{
		`Here you go: {"subject":"kubernetes","content":"The kubernetes cluster runs in Frankfurt and needs weekly node upgrades.","type":"SUMMARY"}`,
	}}

	report, err := m.DistillBrain(context.Background(), "corr-1", &prov, "test-model")
	if err != nil {
		t.Fatalf("DistillBrain: %v", err)
	}
	if report.ClustersFound != 1 || report.ClustersMerged != 1 {
		t.Fatalf("report = %+v", report)
	}
	if report.RecordsSuperseded != 3 {
		t.Fatalf("superseded = %d, want 3", report.RecordsSuperseded)
	}
	if prov.calls != 1 {
		t.Fatalf("provider calls = %d", prov.calls)
	}

	active, err := m.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	// 5 seeded − 3 merged away + 1 consolidated = 3 active.
	if len(active) != 3 {
		t.Fatalf("active = %d (%+v)", len(active), active)
	}
	var consolidated *Record
	for i := range active {
		if active[i].Tags["source"] == "brain-distill" {
			consolidated = &active[i]
		}
	}
	if consolidated == nil {
		t.Fatal("no consolidated record among actives")
	}
	if consolidated.Type != TypeSummary || !strings.Contains(consolidated.Content, "Frankfurt") {
		t.Fatalf("consolidated = %+v", consolidated)
	}
	// Every original points at the successor.
	all, _ := m.All()
	linked := 0
	for _, r := range all {
		if r.SupersededBy == consolidated.ID {
			linked++
		}
	}
	if linked != 3 {
		t.Fatalf("supersede links = %d, want 3", linked)
	}

	// Idempotence: a second pass finds nothing left to merge.
	report2, err := m.DistillBrain(context.Background(), "corr-2", &prov, "test-model")
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if report2.ClustersFound != 0 || prov.calls != 1 {
		t.Fatalf("second pass not idle: %+v calls=%d", report2, prov.calls)
	}
}

func TestDistillBrain_NonJSONSkipsCluster(t *testing.T) {
	m := newConsolidationManager(t)
	for _, r := range consolidationCorpus() {
		if _, _, err := m.Remember("", RememberSpec{Type: r.Type, Subject: r.Subject, Content: r.Content, Tags: r.Tags}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	prov := scriptedProvider{answers: []string{"I would rather chat about the weather."}}
	report, err := m.DistillBrain(context.Background(), "corr", &prov, "m")
	if err != nil {
		t.Fatalf("DistillBrain: %v", err)
	}
	if report.ClustersMerged != 0 || report.SkippedNonJSON != 1 || report.RecordsSuperseded != 0 {
		t.Fatalf("report = %+v", report)
	}
	active, _ := m.Active()
	if len(active) != 5 {
		t.Fatalf("active changed on a skipped cluster: %d", len(active))
	}
}

func TestDistillBrain_RequiresProvider(t *testing.T) {
	m := newConsolidationManager(t)
	if _, err := m.DistillBrain(context.Background(), "c", nil, "m"); err == nil {
		t.Fatal("want error without provider")
	}
}
