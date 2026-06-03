// SPDX-License-Identifier: MIT

package sdk

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
)

// RunInfo is one past agent run's summary, as returned by Runs.
type RunInfo struct {
	// CorrelationID identifies the run.
	CorrelationID string
	// Intent is the task the run was given.
	Intent string
	// Status is the run's terminal state: "completed", "failed", "running", or
	// "abandoned".
	Status string
	// Reason is the failure-reason tag when Status == "failed" (else empty).
	Reason string
	// ParentCorrelation is the lead run's id when this run is a sub-agent (else
	// empty).
	ParentCorrelation string
	// Started is when the run began.
	Started time.Time
	// Duration is how long the run took (zero while still running).
	Duration time.Duration
	// Iterations is the number of agent loop turns.
	Iterations int
	// CostUSD is the run's spend in dollars (0 when unpriced).
	CostUSD float64
	// Model is the run's primary model ("" when unpriced/mock).
	Model string
}

// Runs returns the most recent agent runs, newest first, up to limit (a limit
// <= 0 asks the daemon for its default page). It reads the journal on the
// daemon — no run is started.
func (c *Client) Runs(ctx context.Context, limit int) ([]RunInfo, error) {
	args := map[string]any{}
	if limit > 0 {
		args["limit"] = limit
	}
	res, err := c.cp.Call(ctx, controlplane.CmdRunsList, args)
	if err != nil {
		return nil, err
	}
	return parseRuns(res), nil
}

// parseRuns maps the CmdRunsList result ({"runs":[…]}) to typed RunInfo values,
// converting the wire's unix-ms / ms fields to time.Time / time.Duration.
func parseRuns(res map[string]any) []RunInfo {
	rows, _ := res["runs"].([]any)
	out := make([]RunInfo, 0, len(rows))
	for _, raw := range rows {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		ri := RunInfo{
			Iterations: int(intFromAny(m["iters"])),
			CostUSD:    float64(intFromAny(m["spent_mc"])) / 1e9,
		}
		ri.CorrelationID, _ = m["correlation_id"].(string)
		ri.Intent, _ = m["intent"].(string)
		ri.Status, _ = m["status"].(string)
		ri.Reason, _ = m["reason"].(string)
		ri.ParentCorrelation, _ = m["parent_correlation"].(string)
		ri.Model, _ = m["model"].(string)
		if ms := intFromAny(m["started_unix_ms"]); ms > 0 {
			ri.Started = time.UnixMilli(ms)
		}
		ri.Duration = time.Duration(intFromAny(m["duration_ms"])) * time.Millisecond
		out = append(out, ri)
	}
	return out
}
