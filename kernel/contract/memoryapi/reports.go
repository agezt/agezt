// SPDX-License-Identifier: MIT

package memoryapi

// Provenance: the memory reports (W3.57). Every summary the maintenance and
//             distillation surfaces hand back moved here with the rows they
//             describe: a layer above the module may not name the module, so
//             a report the module alone could build would be a report the
//             app layer could not return.

import "context"

// ProfileReport summarizes one profile-distillation pass.
type ProfileReport struct {
	InputRecords  int      `json:"input_records"`
	FacetsWritten int      `json:"facets_written"`
	Facets        []string `json:"facets,omitempty"`
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

// HygieneStats summarizes the store's health for the maintenance view (M857).
type HygieneStats struct {
	Total      int `json:"total"`
	Active     int `json:"active"`
	Tombstoned int `json:"tombstoned"`
	Superseded int `json:"superseded"`
	Suspended  int `json:"suspended"`
	Expired    int `json:"expired"`
	// Prunable is how many soft-deleted (tombstoned or superseded) records are
	// older than the given cutoff — the dead weight a Prune would reclaim.
	Prunable int `json:"prunable"`
}

// AuditReport is the memory hygiene view used to detect stale or competing
// memories before they contaminate retrieval.
type AuditReport struct {
	Total             int                  `json:"total"`
	Usable            int                  `json:"usable"`
	Suspended         int                  `json:"suspended"`
	Expired           int                  `json:"expired"`
	ContradictionLoad int                  `json:"contradiction_load"`
	ExpiredIDs        []string             `json:"expired_ids,omitempty"`
	SuspendedIDs      []string             `json:"suspended_ids,omitempty"`
	Contradictions    []ContradictionGroup `json:"contradictions,omitempty"`
}

// CleanReport summarizes low-value retention cleanup. Removed means hard-deleted:
// clean targets records that never belonged in memory, not records worth a
// reversible tombstone.
type CleanReport struct {
	DryRun      bool               `json:"dry_run"`
	HardDeleted bool               `json:"hard_deleted"`
	Scanned     int                `json:"scanned"`
	Rejected    int                `json:"rejected"`
	Removed     int                `json:"removed"`
	Decisions   []CleanDecisionRow `json:"decisions,omitempty"`
}

// CleanDecisionRow is one record's fate in a cleanup pass.
type CleanDecisionRow struct {
	ID      string `json:"id"`
	Subject string `json:"subject,omitempty"`
	Reason  string `json:"reason"`
}

// ContradictionGroup is a set of records that compete with one another.
type ContradictionGroup struct {
	Key     string   `json:"key"`
	IDs     []string `json:"ids"`
	Subject string   `json:"subject,omitempty"`
	Type    Type     `json:"type,omitempty"`
	Scope   string   `json:"scope,omitempty"`
}

// RetentionDecision explains whether a record belongs in long-term memory.
// Journal/log output belongs in the journal, not here.
type RetentionDecision struct {
	Keep   bool   `json:"keep"`
	Reason string `json:"reason,omitempty"`
}

// Embedder turns texts into semantic vectors. Implementations MUST return
// exactly one L2-normalized vector per input text, all of one consistent
// dimensionality, in input order. Batch + ctx + error fit a remote API's
// reality (one round trip per recall, cancellable, fallible).
type Embedder interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}
