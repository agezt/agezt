// SPDX-License-Identifier: MIT

// introspect_views.go owns the per-record render helpers
// used by Tool.Invoke to flatten cadence.Entry /
// standing.Order rows for JSON output, plus the
// okJSON / errResult formatters. The Tool type + its
// agent.Tool surface live in introspect.go.
package introspecttool

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/standing"
)

// cadence string plus the load-bearing fields, omitting zero/empty noise.
func scheduleView(e cadence.Entry) map[string]any {
	v := map[string]any{
		"id":            e.ID,
		"intent":        e.Intent,
		"cadence":       e.Cadence(),
		"mode":          e.Mode,
		"source":        e.Source,
		"enabled":       e.Enabled,
		"next_run_unix": e.NextRunUnix,
	}
	if e.LastRunUnix > 0 {
		v["last_run_unix"] = e.LastRunUnix
	}
	if e.Fires > 0 {
		v["fires"] = e.Fires
	}
	if e.Model != "" {
		v["model"] = e.Model
	}
	if e.Assure > 0 {
		v["assure"] = e.Assure
	}
	return v
}

func next(v map[string]any) int64 {
	if n, ok := v["next_run_unix"].(int64); ok {
		return n
	}
	return 0
}

// standingView renders one standing order — mirrors the standing tool's view so
// the two read identically.
func standingView(o standing.Order) map[string]any {
	trigs := make([]map[string]any, 0, len(o.Triggers))
	for _, tr := range o.Triggers {
		m := map[string]any{"type": string(tr.Type)}
		if tr.Schedule != "" {
			m["schedule"] = tr.Schedule
		}
		if tr.Subject != "" {
			m["subject"] = tr.Subject
		}
		trigs = append(trigs, m)
	}
	v := map[string]any{
		"id": o.ID, "name": o.Name, "enabled": o.Enabled,
		"mode": string(o.Initiative.Mode), "triggers": trigs, "plan": o.Plan,
	}
	if o.Assure > 0 {
		v["assure"] = o.Assure
	}
	return v
}

func okJSON(v any) agent.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "introspect: " + msg, IsError: true}
}
