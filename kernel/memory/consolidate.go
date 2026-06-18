// SPDX-License-Identifier: MIT

package memory

// Brain distillation (M804, vision gap #6): the per-run auto-distiller
// (Distill) extracts facts as tasks complete; over weeks that accretes many
// small, overlapping records about the same things. Consolidation is the
// complementary "sleep cycle": cluster related records by embedding cosine
// (the M803 local vectors), have the LLM merge each cluster into ONE
// concise record, and supersede the originals — soft, journaled, reversible,
// exactly like every other memory mutation. The store shrinks to its
// essence; nothing is destroyed.
//
// Scope is a hard wall: records private to different scopes never share a
// cluster, and a cluster's consolidated record keeps its scope — a private
// note can never leak into shared memory by being "summarized into" it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// Clustering / pass bounds. Threshold 0.55 under hashed-n-gram cosine means
// "clearly about the same thing" (near-duplicates and re-extractions land
// 0.6+, mere topic neighbours ~0.3). A pass consolidates at most
// maxClustersPerPass clusters so one timer tick stays cheap and incremental.
const (
	clusterCosine      = 0.55
	minClusterSize     = 3
	maxClustersPerPass = 4
	maxClusterRecords  = 12 // prompt-size guard: merge the 12 oldest, leave the rest for the next pass
)

// Clusters groups active records into same-scope clusters of embedding
// neighbours: each record (in deterministic store order) joins the first
// cluster whose SEED it matches at ≥ threshold, else seeds a new cluster.
// Seed-matching (not centroid) keeps the function pure and order-stable.
// Only clusters of at least minSize survive.
//
// Performance: O(n) overall. Records are first partitioned by scope (hard wall),
// then clustered within each scope. The inner cluster scan is bounded by
// maxClusterRecords since scope partitioning prevents cross-scope growth.
func Clusters(rs []Record, threshold float64, minSize int) [][]Record {
	type cluster struct {
		seedVec []float32
		scope   string
		recs    []Record
	}

	// Phase 1: Partition by scope — O(n). Scope is a hard wall; records from
	// different scopes never cluster together.
	byScope := make(map[string][]Record)
	for _, r := range rs {
		if !r.Active() || r.Suspended() {
			continue
		}
		scope := ""
		if r.Tags != nil {
			scope = r.Tags["scope"]
		}
		byScope[scope] = append(byScope[scope], r)
	}

	// Phase 2: Cluster within each scope. Each scope is independent.
	// We maintain a map of scope -> clusters for that scope only.
	// This means when searching for a match we only check clusters from
	// the SAME scope, bounding the inner loop to O(k) where k is the
	// number of clusters within that scope (at most maxClusterRecords).
	scopeClusters := make(map[string][]*cluster)
	for _, recs := range byScope {
		sortRecords(recs)
		for _, r := range recs {
			rv := Embed(searchText(r))
			if rv == nil {
				continue
			}
			scope := ""
			if r.Tags != nil {
				scope = r.Tags["scope"]
			}
			clusters := scopeClusters[scope]
			placed := false
			for _, c := range clusters {
				if Cosine(c.seedVec, rv) >= threshold {
					c.recs = append(c.recs, r)
					placed = true
					break
				}
			}
			if !placed {
				scopeClusters[scope] = append(clusters, &cluster{seedVec: rv, scope: scope, recs: []Record{r}})
			}
		}
	}

	// Collect all clusters meeting minimum size
	out := make([][]Record, 0)
	for _, clusters := range scopeClusters {
		for _, c := range clusters {
			if len(c.recs) >= minSize {
				out = append(out, c.recs)
			}
		}
	}
	return out
}

// consolidateSystem instructs the provider to merge related records into one.
// Strict JSON keeps parsing deterministic; a non-JSON answer skips the
// cluster (the best-effort contract — consolidation never fails the pass).
const consolidateSystem = `You consolidate an agent's memory. You are given several related memory records about the same topic. ` +
	`Merge them into ONE concise record that preserves every durable fact and drops repetition and stale phrasing. ` +
	`Return ONLY a JSON object: {"subject":"...","content":"...","type":"FACT|SUMMARY|PREFERENCE"}. ` +
	`The content must stand alone (no references to "the records above").`

type consolidateResult struct {
	Subject string `json:"subject"`
	Content string `json:"content"`
	Type    Type   `json:"type"`
}

// BrainDistillReport summarizes one consolidation pass.
type BrainDistillReport struct {
	ClustersFound     int      `json:"clusters_found"`
	ClustersMerged    int      `json:"clusters_merged"`
	RecordsSuperseded int      `json:"records_superseded"`
	ConsolidatedIDs   []string `json:"consolidated_ids,omitempty"`
	SkippedNonJSON    int      `json:"skipped_non_json,omitempty"`
	ActiveBefore      int      `json:"active_before"`
	ActiveAfterApprox int      `json:"active_after_approx"`
}

// DistillBrain runs one consolidation pass: cluster the active records,
// merge up to maxClustersPerPass clusters through the provider, supersede
// the originals, and journal one memory.consolidated event with the counts.
// Best-effort per cluster: a provider error aborts the pass (budget/network
// problems shouldn't burn more calls), a non-JSON answer just skips that
// cluster.
func (m *Manager) DistillBrain(ctx context.Context, corr string, provider agent.Provider, model string) (BrainDistillReport, error) {
	if provider == nil {
		return BrainDistillReport{}, errors.New("memory: brain distill requires a provider")
	}
	active, err := m.Active()
	if err != nil {
		return BrainDistillReport{}, err
	}
	report := BrainDistillReport{ActiveBefore: len(active)}
	clusters := Clusters(active, clusterCosine, minClusterSize)
	report.ClustersFound = len(clusters)
	if len(clusters) > maxClustersPerPass {
		clusters = clusters[:maxClustersPerPass]
	}
	for _, cluster := range clusters {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if len(cluster) > maxClusterRecords {
			cluster = cluster[:maxClusterRecords]
		}
		merged, ok, err := m.consolidateCluster(ctx, corr, provider, model, cluster)
		if err != nil {
			return report, err
		}
		if !ok {
			report.SkippedNonJSON++
			continue
		}
		report.ClustersMerged++
		report.ConsolidatedIDs = append(report.ConsolidatedIDs, merged.ID)
		for _, old := range cluster {
			if old.ID == merged.ID {
				continue // content-addressing: the merge may equal an existing record
			}
			if err := m.supersedeExisting(corr, old.ID, merged.ID); err != nil {
				return report, err
			}
			report.RecordsSuperseded++
		}
	}
	report.ActiveAfterApprox = report.ActiveBefore - report.RecordsSuperseded + report.ClustersMerged
	if report.ClustersMerged > 0 || report.ClustersFound > 0 {
		m.publish(event.KindMemoryConsolidated, corr, map[string]any{
			"clusters_found":     report.ClustersFound,
			"clusters_merged":    report.ClustersMerged,
			"records_superseded": report.RecordsSuperseded,
			"ids":                report.ConsolidatedIDs,
		})
	}
	return report, nil
}

// consolidateCluster merges one cluster through the provider. ok=false means
// the answer wasn't usable (skip, don't fail).
func (m *Manager) consolidateCluster(ctx context.Context, corr string, provider agent.Provider, model string, cluster []Record) (Record, bool, error) {
	var b strings.Builder
	for i, r := range cluster {
		fmt.Fprintf(&b, "%d. [%s] %s: %s\n", i+1, r.Type, r.Subject, r.Content)
	}
	resp, err := provider.Complete(ctx, agent.CompletionRequest{
		Model:    model,
		System:   consolidateSystem,
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "Related memory records:\n" + b.String()}},
		TaskType: "distill", // same budgeting/routing class as per-run distillation
	})
	if err != nil {
		return Record{}, false, fmt.Errorf("memory: consolidate completion: %w", err)
	}
	start := strings.IndexByte(resp.Message.Content, '{')
	end := strings.LastIndexByte(resp.Message.Content, '}')
	if start < 0 || end <= start {
		return Record{}, false, nil
	}
	var parsed consolidateResult
	if err := json.Unmarshal([]byte(resp.Message.Content[start:end+1]), &parsed); err != nil {
		return Record{}, false, nil
	}
	if strings.TrimSpace(parsed.Content) == "" {
		return Record{}, false, nil
	}
	t := parsed.Type
	if !ValidType(t) {
		t = TypeSummary
	}
	tags := map[string]string{"source": "brain-distill"}
	// The scope wall: the consolidated record inherits the cluster's scope.
	if s := cluster[0].Tags["scope"]; s != "" {
		tags["scope"] = s
	}
	rec, _, err := m.Remember(corr, RememberSpec{
		Type:    t,
		Subject: parsed.Subject,
		Content: parsed.Content,
		Tags:    tags,
	})
	if err != nil {
		return Record{}, false, err
	}
	return rec, true, nil
}

// supersedeExisting links an existing record to its consolidated successor
// (the Supersede method creates the successor too; here it already exists).
func (m *Manager) supersedeExisting(corr, oldID, newID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, found, err := m.store.Get(oldID)
	if err != nil || !found {
		return err
	}
	if old.SupersededBy != "" {
		return nil // already linked (idempotent across overlapping passes)
	}
	old.SupersededBy = newID
	old.LastSeenMS = m.now().UnixMilli()
	if err := m.store.Put(old); err != nil {
		return err
	}
	m.publish(event.KindMemorySuperseded, corr, map[string]any{
		"old_id": oldID,
		"new_id": newID,
	})
	return nil
}
