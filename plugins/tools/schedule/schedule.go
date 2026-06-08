// SPDX-License-Identifier: MIT

// Package schedule is the in-process self-scheduling tool. It lets the agent
// arrange its OWN future work — "remind me / do this later / every morning" —
// by writing to the daemon's persistent cadence store, the same store the
// `agt schedule` CLI and the AGEZT_SCHEDULE env jobs use. A scheduled intent
// fires through the normal governed loop at its due time (M634).
//
// This is the autonomy primitive that turns a reactive assistant into a
// proactive one: the agent can say "I'll check back in an hour" and actually
// have it happen. Schedules it creates are tagged source="agent" so an operator
// can see and prune them (`agt schedule list`).
//
// The tool is created unbound and Bound to the live store after the kernel
// opens (the store is the kernel's), mirroring the notify tool's lifecycle.
package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/cadence"
)

// store is the subset of *cadence.Store the tool needs — an interface so tests
// can inject a fake without a real on-disk store.
type store interface {
	Add(intent string, interval time.Duration, model, source string, now time.Time) (cadence.Entry, error)
	AddDaily(intent string, atMinutes, days int, tz, model, source string, now time.Time) (cadence.Entry, error)
	AddOnce(intent string, at time.Time, model, source string, now time.Time) (cadence.Entry, error)
	Remove(id string) (bool, error)
	List() []cadence.Entry
}

// Tool implements agent.Tool. Created unbound via New(); Bind wires the store.
type Tool struct {
	mu    sync.RWMutex
	store store
	now   func() time.Time
}

// New returns an unbound schedule tool (no store until Bind).
func New() *Tool { return &Tool{now: time.Now} }

// Bind wires the live cadence store. Called once after the kernel opens.
func (t *Tool) Bind(s *cadence.Store) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s != nil {
		t.store = s
	}
}

func (t *Tool) current() (store, func() time.Time) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := t.now
	if now == nil {
		now = time.Now
	}
	return t.store, now
}

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "schedule",
		Description: "Schedule your OWN future work: run an intent later (once after a " +
			"delay, every interval, or daily at a wall-clock time), or list/remove your " +
			"schedules. The intent fires as a fresh run at its due time. Use this to set " +
			"reminders or recurring checks instead of asking the user to come back.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":       {"type":"string", "enum":["in","every","daily","list","remove"], "description":"in=one-shot after a delay; every=recurring interval; daily=at a wall-clock time; list; remove."},
    "intent":   {"type":"string", "description":"The task to run at the scheduled time (for in/every/daily)."},
    "delay":    {"type":"string", "description":"For op=in: how far out, e.g. \"30m\", \"2h\", \"24h\"."},
    "interval": {"type":"string", "description":"For op=every: the firing period, e.g. \"1h\", \"15m\"."},
    "at":       {"type":"string", "description":"For op=daily: wall-clock time \"HH:MM\" (24h, daemon local time)."},
    "days":     {"type":"string", "description":"For op=daily (optional): which days, e.g. \"mon-fri\", \"weekends\". Default every day."},
    "model":    {"type":"string", "description":"Optional model override for the scheduled run."},
    "id":       {"type":"string", "description":"For op=remove: the schedule id to delete."}
  }
}`),
	}
}

type input struct {
	Op       string `json:"op"`
	Intent   string `json:"intent"`
	Delay    string `json:"delay"`
	Interval string `json:"interval"`
	At       string `json:"at"`
	Days     string `json:"days"`
	Model    string `json:"model"`
	ID       string `json:"id"`
}

const source = "agent" // marks schedules the agent created, for operator visibility

// Invoke implements agent.Tool.
func (t *Tool) Invoke(_ context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("schedule: parse input: %w", err)
	}
	st, nowFn := t.current()
	if st == nil {
		return errResult("scheduling is not available on this daemon"), nil
	}
	now := nowFn()

	switch in.Op {
	case "in":
		d, err := time.ParseDuration(in.Delay)
		if err != nil || d <= 0 {
			return errResult(`op=in needs a positive "delay" duration like "30m" or "2h"`), nil
		}
		e, err := st.AddOnce(in.Intent, now.Add(d), in.Model, source, now)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okEntry("scheduled once", e), nil

	case "every":
		d, err := time.ParseDuration(in.Interval)
		if err != nil || d <= 0 {
			return errResult(`op=every needs a positive "interval" like "1h" or "15m"`), nil
		}
		e, err := st.Add(in.Intent, d, in.Model, source, now)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okEntry("scheduled recurring", e), nil

	case "daily":
		mins, ok := parseHHMM(in.At)
		if !ok {
			return errResult(`op=daily needs an "at" time in HH:MM (24h)`), nil
		}
		days := 0 // 0 = every day
		if in.Days != "" {
			d, err := cadence.ParseDays(in.Days)
			if err != nil {
				return errResult("bad days spec: " + err.Error()), nil
			}
			days = d
		}
		e, err := st.AddDaily(in.Intent, mins, days, "", in.Model, source, now)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okEntry("scheduled daily", e), nil

	case "remove":
		if in.ID == "" {
			return errResult(`op=remove needs an "id"`), nil
		}
		removed, err := st.Remove(in.ID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if !removed {
			return errResult("no schedule with id " + in.ID), nil
		}
		return okJSON(map[string]any{"removed": in.ID}), nil

	case "list":
		entries := st.List()
		out := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			out = append(out, entryView(e))
		}
		return okJSON(map[string]any{"count": len(out), "schedules": out}), nil

	case "":
		return errResult("op required (in|every|daily|list|remove)"), nil
	default:
		return errResult("unknown op " + in.Op + " (in|every|daily|list|remove)"), nil
	}
}

// parseHHMM parses "HH:MM" (24h) into minutes since midnight. Returns ok=false
// on any malformed input.
func parseHHMM(s string) (int, bool) {
	var h, m int
	if n, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || n != 2 {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func entryView(e cadence.Entry) map[string]any {
	v := map[string]any{
		"id":      e.ID,
		"intent":  e.Intent,
		"cadence": e.Cadence(),
		"enabled": e.Enabled,
		"source":  e.Source,
	}
	if e.NextRunUnix > 0 {
		v["next_run"] = time.Unix(e.NextRunUnix, 0).Format(time.RFC3339)
	}
	return v
}

func okEntry(msg string, e cadence.Entry) agent.Result {
	view := entryView(e)
	view["message"] = msg
	return okJSON(view)
}

func okJSON(v any) agent.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "schedule: " + msg, IsError: true}
}
