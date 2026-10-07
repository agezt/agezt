// SPDX-License-Identifier: MIT
package tools

import "github.com/agezt/agezt/kernel/platform/journalview"

type LogItem struct {
	Actor             string   `json:"actor"`
	CorrelationID     string   `json:"correlation_id"`
	Tool              string   `json:"tool"`
	CallID            string   `json:"call_id"`
	Input             string   `json:"input"`
	Output            string   `json:"output"`
	Error             bool     `json:"error"`
	DurationMS        int64    `json:"duration_ms"`
	ObservationTrust  string   `json:"observation_trust"`
	ObservationSource string   `json:"observation_source"`
	DirectiveLike     bool     `json:"directive_like"`
	DirectiveMatches  []string `json:"directive_matches"`
	Seq               int64    `json:"seq"`
	TSUnixMS          int64    `json:"ts_unix_ms"`
	NotExecuted       bool     `json:"not_executed,omitempty"`
}
type ToolSummary struct {
	Calls  int    `json:"calls"`
	Errors int    `json:"errors"`
	AvgMS  *int64 `json:"avg_ms,omitempty"`
}
type LatencySummary struct {
	Count int `json:"count"`
	journalview.DurationStats
}
