// SPDX-License-Identifier: MIT

// Schedule tool: types + lifecycle + Definition + Invoke + view/ok helpers.
// Code extracted from schedule.go during the Day-100 god-file split.
// Public API unchanged.
package schedule


import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/roster"
)


// store is the subset of *cadence.Store the tool needs — an interface so tests
// can inject a fake without a real on-disk store.
type store interface {
	Add(intent string, interval time.Duration, model, source string, now time.Time) (cadence.Entry, error)
	AddDaily(intent string, atMinutes, days int, tz, model, source string, now time.Time) (cadence.Entry, error)
	AddOnce(intent string, at time.Time, model, source string, now time.Time) (cadence.Entry, error)
	AddContinuous(intent string, cooldown time.Duration, model, source string, now time.Time) (cadence.Entry, error)
	SetAssure(id string, n int) (bool, error)
	SetAgent(id, agent string) (bool, error)
	SetWorkflowTarget(id, ref string, payload json.RawMessage) (bool, error)
	SetSystemTaskTarget(id, task string) (bool, error)
	SetToolTarget(id, tool string, payload json.RawMessage) (bool, error)
	Remove(id string) (bool, error)
	List() []cadence.Entry
}

// Tool implements agent.Tool. Created unbound via New(); Bind wires the store.
type Tool struct {
	mu          sync.RWMutex
	store       store
	now         func() time.Time
	agentLookup func(string) (roster.Profile, bool)
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

func (t *Tool) BindAgentLookup(lookup func(string) (roster.Profile, bool)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.agentLookup = lookup
}

func (t *Tool) current() (store, func() time.Time, func(string) (roster.Profile, bool)) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := t.now
	if now == nil {
		now = time.Now
	}
	return t.store, now, t.agentLookup
}

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	systemTaskEnum, _ := json.Marshal(cadence.SystemTasks())
	return agent.ToolDef{
		Name:       "schedule",
		Capability: agent.ToolCapability{Name: string(edict.CapSchedule)},
		Description: "Schedule future work: run your own agent task later, wake a workflow, " +
			"run a system task, or invoke a registered tool on a cadence. Use typed targets " +
			"instead of embedding execution instructions in the task/label.",
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"create, list, or remove future scheduled jobs",
				"created schedules will launch typed cron jobs later until removed",
			},
			AffectedResources: []string{"cadence schedule store"},
			RollbackNotes:     "Created schedules can be removed by id with op=remove or via the operator schedule UI/CLI.",
			Confidence:        0.9,
		},
		InputSchema: json.RawMessage(fmt.Sprintf(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":       {"type":"string", "enum":["in","every","daily","continuous","list","remove"], "description":"in=one-shot after a delay; every=recurring interval; daily=at a wall-clock time; continuous=a never-ending loop that re-runs after each run finishes; list; remove."},
    "intent":   {"type":"string", "description":"For target=agent/intent: the task to run at the scheduled time. For workflow/system_task/tool, optional label only, not instructions."},
    "target":   {"type":"string", "enum":["agent","intent","workflow","system_task","tool"], "description":"What this schedule fires. Default agent/intent runs this agent's own task. workflow runs a stored workflow. system_task runs daemon maintenance. tool invokes a registered tool."},
    "workflow": {"type":"string", "description":"Workflow name/id when target=workflow."},
    "system_task": {"type":"string", "enum":%s, "description":"System maintenance task when target=system_task."},
    "tool":     {"type":"string", "description":"Registered tool name when target=tool."},
    "payload":  {"description":"JSON payload for target=workflow or target=tool."},
    "delay":    {"type":"string", "description":"For op=in: how far out, e.g. \"30m\", \"2h\", \"24h\"."},
    "interval": {"type":"string", "description":"For op=every: the firing period, e.g. \"1h\", \"15m\"."},
    "cooldown": {"type":"string", "description":"For op=continuous: the breather between cycles, e.g. \"30s\", \"5m\". The loop runs forever; pause/remove it to stop."},
    "at":       {"type":"string", "description":"For op=daily: wall-clock time \"HH:MM\" (24h, daemon local time)."},
    "days":     {"type":"string", "description":"For op=daily (optional): which days, e.g. \"mon-fri\", \"weekends\". Default every day."},
    "model":    {"type":"string", "description":"Optional model override for the scheduled run."},
    "assure":   {"type":"integer", "description":"Optional do-it-for-sure budget for in/every/daily/continuous: if > 0, each firing runs, verifies it was actually completed, and retries the gap up to this many attempts. Use it for tasks that must definitely get done."},
    "id":       {"type":"string", "description":"For op=remove: the schedule id to delete."}
  }
}`, string(systemTaskEnum))),
	}
}

type input struct {
	Op       string `json:"op"`
	Intent   string `json:"intent"`
	Delay    string `json:"delay"`
	Interval string `json:"interval"`
	Cooldown string `json:"cooldown"`
	At       string `json:"at"`
	Days     string `json:"days"`
	Model    string `json:"model"`
	ID       string `json:"id"`
	Assure   int    `json:"assure"`
	Target   string `json:"target"`
	Workflow string `json:"workflow"`
	System   string `json:"system_task"`
	Tool     string `json:"tool"`
	Payload  any    `json:"payload"`
}

const source = "agent" // marks schedules the agent created, for operator visibility

// applyAssure stamps a do-it-for-sure budget onto a freshly created entry, so
// each firing runs-verifies-retries up to n attempts (M654). Best-effort: a
// SetAssure failure leaves the entry a single-pass schedule (no worse than
// assure being unset). Returns the entry with Assure reflected for display.
func applyAssure(st store, e cadence.Entry, n int) cadence.Entry {
	if n > 0 {
		if _, err := st.SetAssure(e.ID, n); err == nil {
			e.Assure = n
		}
	}
	return e
}

func applyActingAgent(ctx context.Context, st store, e cadence.Entry) cadence.Entry {
	if slug := agent.AgentFromContext(ctx); slug != "" {
		if _, err := st.SetAgent(e.ID, slug); err == nil {
			e.Agent = slug
		}
	}
	return e
}

func scheduleBindsActingAgent(in input) bool {
	target := strings.TrimSpace(in.Target)
	if target == "" {
		return strings.TrimSpace(in.Workflow) == "" && strings.TrimSpace(in.System) == "" && strings.TrimSpace(in.Tool) == ""
	}
	return target == "agent" || target == cadence.TargetIntent || target == cadence.TargetWorkflow || target == cadence.TargetTool
}

func validateActingAgentSchedule(ctx context.Context, in input, lookup func(string) (roster.Profile, bool)) agent.Result {
	if lookup == nil || !scheduleBindsActingAgent(in) {
		return agent.Result{}
	}
	slug := strings.TrimSpace(agent.AgentFromContext(ctx))
	if slug == "" {
		return agent.Result{}
	}
	p, ok := lookup(slug)
	if !ok {
		return errResult("acting agent " + slug + " is not in the roster")
	}
	if p.Retired {
		return errResult("agent " + p.Slug + " is retired and cannot schedule a direct wake")
	}
	if !p.Enabled {
		return errResult("agent " + p.Slug + " is paused and cannot schedule a direct wake")
	}
	if !p.AllowsDirectCall() {
		return errResult(managedSubAgentScheduleHint(p))
	}
	return agent.Result{}
}

func managedSubAgentScheduleHint(p roster.Profile) string {
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	hint := "route the work through its parent/owner agent"
	if manager != "" {
		hint = "wake " + manager + " or delegate through it"
	}
	return "agent " + p.Slug + " is a managed sub-agent and cannot create independently firing schedules; " + hint
}

func scheduleTarget(in input) string {
	target := strings.TrimSpace(in.Target)
	if target == "" {
		switch {
		case strings.TrimSpace(in.Workflow) != "":
			target = cadence.TargetWorkflow
		case strings.TrimSpace(in.System) != "":
			target = cadence.TargetSystemTask
		case strings.TrimSpace(in.Tool) != "":
			target = cadence.TargetTool
		default:
			target = cadence.TargetIntent
		}
	}
	if target == "agent" {
		return cadence.TargetIntent
	}
	return target
}

