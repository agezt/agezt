// SPDX-License-Identifier: MIT

package providers

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
)

// Observations reads the journal selected by the host's tenant routing.
type Observations struct{ journal journalview.Reader }

func NewObservations(reader journalview.Reader) *Observations { return &Observations{journal: reader} }

// ObservationInput contains adapter-admitted limits and an absolute event cutoff.
// Limit must be positive; WindowMS retains the requested relative window in stats.
type ObservationInput struct {
	Limit              int
	CutoffMS, WindowMS int64
	Cursor             any
	FallbacksOnly      bool
}

func (s *Observations) Stats(in ObservationInput) (map[string]any, error) {
	var routed, fallbacks int
	byPrimary := map[string]int{}        // routing.decision primary → count
	fallbackByFailed := map[string]int{} // provider.fallback failed → count
	if err := s.journal.Range(func(e *event.Event) error {
		if in.CutoffMS > 0 && e.TSUnixMS < in.CutoffMS {
			return nil
		}
		switch e.Kind {
		case event.KindRoutingDecision:
			var p struct {
				Primary string `json:"primary"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			routed++
			if p.Primary != "" {
				byPrimary[p.Primary]++
			}
		case event.KindProviderFallback:
			var p struct {
				Failed string `json:"failed"`
				Scope  string `json:"scope"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			// Model-chain fallbacks (M706, scope="model-chain") are a different
			// dimension (model→model, surfaced in the Routing view); don't conflate
			// them into the provider fallback rate, which measures provider→provider.
			if p.Scope == "model-chain" {
				return nil
			}
			fallbacks++
			if p.Failed != "" {
				fallbackByFailed[p.Failed]++
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	fallbackRate := 0.0
	if routed > 0 {
		fallbackRate = float64(fallbacks) / float64(routed)
	}
	byPrimaryOut := make(map[string]any, len(byPrimary))
	for n, c := range byPrimary {
		byPrimaryOut[n] = c
	}
	fbOut := make(map[string]any, len(fallbackByFailed))
	for n, c := range fallbackByFailed {
		fbOut[n] = c
	}
	return map[string]any{"routed": routed, "fallbacks": fallbacks, "fallback_rate": fallbackRate, "by_primary": byPrimaryOut, "fallbacks_by_primary": fbOut, "window_ms": in.WindowMS}, nil
}

func (s *Observations) Rejections(in ObservationInput) (map[string]any, error) {
	type capEvent struct {
		ts, seq            int64
		kind               string // rejected | rerouted
		capability, model  string
		fromModel, toModel string
	}
	rows := make([]capEvent, 0)
	if err := s.journal.Range(func(e *event.Event) error {
		if in.CutoffMS > 0 && e.TSUnixMS < in.CutoffMS {
			return nil
		}
		switch e.Kind {
		case event.KindCapabilityRejected:
			var p struct{ Model, Capability string }
			_ = json.Unmarshal(e.Payload, &p)
			rows = append(rows, capEvent{ts: e.TSUnixMS, seq: e.Seq, kind: "rejected", capability: p.Capability, model: p.Model})
		case event.KindCapabilityRerouted:
			var p struct {
				FromModel  string `json:"from_model"`
				ToModel    string `json:"to_model"`
				Capability string `json:"capability"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			rows = append(rows, capEvent{ts: e.TSUnixMS, seq: e.Seq, kind: "rerouted", capability: p.Capability, fromModel: p.FromModel, toModel: p.ToModel})
		}
		return nil
	}); err != nil {
		return nil, err
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ts != rows[j].ts {
			return rows[i].ts > rows[j].ts
		}
		return rows[i].seq > rows[j].seq
	})
	if len(rows) > in.Limit {
		rows = rows[:in.Limit]
	}

	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		row := map[string]any{"ts_unix_ms": r.ts, "kind": r.kind, "capability": r.capability}
		if r.kind == "rerouted" {
			row["from_model"] = r.fromModel
			row["to_model"] = r.toModel
		} else {
			row["model"] = r.model
		}
		out = append(out, row)
	}
	return map[string]any{"rejections": out, "count": len(out)}, nil
}

func (s *Observations) Log(in ObservationInput) (map[string]any, error) {
	output, err := journalview.Project(s.journal, journalview.Input{Limit: in.Limit, CutoffMS: in.CutoffMS, Cursor: in.Cursor}, func(e *event.Event) (map[string]any, bool) {
		switch e.Kind {
		case event.KindRoutingDecision:
			if in.FallbacksOnly {
				return nil, false
			}
			var p struct {
				Primary  string   `json:"primary"`
				Chain    []string `json:"chain"`
				TaskType string   `json:"task_type"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			return map[string]any{
				"kind": "route", "primary": p.Primary,
				"chain": strings.Join(p.Chain, ","), "task_type": p.TaskType,
			}, true
		case event.KindProviderFallback:
			var p struct {
				Failed, Next, Reason string
				FailedModel          string `json:"failed_model"`
				NextModel            string `json:"next_model"`
				Scope                string `json:"scope"`
				TaskType             string `json:"task_type"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			// Model-chain fallbacks (M706) name the failed/next MODEL, not provider;
			// normalise into the same failed/next fields so the timeline shows the hop.
			row := map[string]any{"kind": "fallback", "reason": p.Reason}
			if p.Scope != "" {
				row["scope"] = p.Scope
			}
			if p.Scope == "model-chain" {
				row["failed"], row["next"], row["task_type"] = p.FailedModel, p.NextModel, p.TaskType
			} else {
				row["failed"], row["next"] = p.Failed, p.Next
			}
			return row, true
		default:
			return nil, false
		}
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"events": output.Rows, "count": output.Count, "next_cursor": output.NextCursor}, nil
}
