// SPDX-License-Identifier: MIT

package workflow

// Template gallery (M807): curated, ready-to-run starting points — one per
// engine capability worth learning (cron+branching, event+HITL, the error
// port, switch+merge joins, map/filter pipelines). Templates ship in the
// binary (no network, no marketplace dependency), every one passes the SAME
// Validate the save path uses (pinned by test), and instantiating one is
// just opening it on the canvas under a new name — the gallery never writes
// to the store by itself.

import "encoding/json"

// Template is one gallery entry. The embedded workflow's Name is a slug
// suggestion; instantiation renames it.
type Template struct {
	Name        string   `json:"name"`  // gallery slug
	Title       string   `json:"title"` // human title
	Description string   `json:"description"`
	Category    string   `json:"category"` // monitor | ops | data | demo
	Workflow    Workflow `json:"workflow"`
}

// raw builds a config payload; templates are authored in Go so a typo is a
// compile error, and TestTemplatesValidate pins schema drift.
func raw(s string) json.RawMessage { return json.RawMessage(s) }

// Templates returns the built-in gallery. The returned slice is freshly
// allocated; callers may mutate their copy.
func Templates() []Template {
	return []Template{
		{
			Name:        "daily-status-check",
			Title:       "Daily status check",
			Description: "Every morning, GET a status URL; if the body doesn't contain OK, have the LLM write an alert — otherwise report all-good.",
			Category:    "monitor",
			Workflow: Workflow{
				Name:        "daily-status-check",
				Description: "cron → http → condition → llm alert | all-good",
				Nodes: []Node{
					{ID: "start", Type: NodeTrigger, Label: "Daily 09:00", Config: raw(`{"kind":"cron","daily_at":"09:00"}`), X: 40, Y: 40},
					{ID: "fetch", Type: NodeHTTP, Label: "GET status page", Config: raw(`{"method":"GET","url":"https://example.com/status"}`), X: 40, Y: 190},
					{ID: "is_ok", Type: NodeCondition, Label: "Contains OK?", Config: raw(`{"left":"{{fetch.output.body}}","op":"contains","right":"OK"}`), X: 40, Y: 340},
					{ID: "all_good", Type: NodeTransform, Label: "All good", Config: raw(`{"template":"status OK at {{trigger.payload.fired_at}}"}`), X: -80, Y: 490},
					{ID: "alert", Type: NodeLLM, Label: "Write alert", Config: raw(`{"prompt":"The status page did not report OK. Body was: {{fetch.output.body}}. Write a one-line ops alert."}`), X: 160, Y: 490},
				},
				Edges: []Edge{
					{From: "start", To: "fetch"},
					{From: "fetch", To: "is_ok"},
					{From: "is_ok", To: "all_good", Port: "true"},
					{From: "is_ok", To: "alert", Port: "false"},
				},
			},
		},
		{
			Name:        "failed-task-triage",
			Title:       "Failed-task triage",
			Description: "When any task fails, summarize the event, have the LLM suggest a one-line fix, and gate the conclusion behind a human approval.",
			Category:    "ops",
			Workflow: Workflow{
				Name:        "failed-task-triage",
				Description: "event task.failed → summary → llm fix → approval",
				Nodes: []Node{
					{ID: "start", Type: NodeTrigger, Label: "On task.failed", Config: raw(`{"kind":"event","subject":"task.failed"}`), X: 40, Y: 40},
					{ID: "summary", Type: NodeTransform, Label: "Summarize failure", Config: raw(`{"template":"task failed: {{trigger.payload.event}} — {{trigger.payload.data}}"}`), X: 40, Y: 190},
					{ID: "fix", Type: NodeLLM, Label: "Suggest fix", Config: raw(`{"prompt":"Given this failure, suggest a one-line fix: {{summary.output}}"}`), X: 40, Y: 340},
					{ID: "gate", Type: NodeApproval, Label: "Human sign-off", Config: raw(`{"description":"Triage suggestion ready: {{fix.output}} — accept?"}`), X: 40, Y: 490},
					{ID: "done", Type: NodeTransform, Label: "Conclusion", Config: raw(`{"template":"triage accepted: {{fix.output}}"}`), X: 40, Y: 640},
				},
				Edges: []Edge{
					{From: "start", To: "summary"},
					{From: "summary", To: "fix"},
					{From: "fix", To: "gate"},
					{From: "gate", To: "done"},
				},
			},
		},
		{
			Name:        "resilient-fetch",
			Title:       "Resilient fetch (error port)",
			Description: "Fetch a URL; on failure the error port rescues the run — the error message flows into a fallback branch instead of failing the workflow.",
			Category:    "demo",
			Workflow: Workflow{
				Name:        "resilient-fetch",
				Description: "http with an error-port rescue branch",
				Nodes: []Node{
					{ID: "start", Type: NodeTrigger, Label: "Manual", Config: raw(`{"kind":"manual"}`), X: 40, Y: 40},
					{ID: "fetch", Type: NodeHTTP, Label: "GET (may fail)", Config: raw(`{"method":"GET","url":"{{trigger.payload.url}}"}`), X: 40, Y: 190},
					{ID: "shape", Type: NodeTransform, Label: "Happy path", Config: raw(`{"template":"fetched {{fetch.output.status}}"}`), X: -80, Y: 340},
					{ID: "rescue", Type: NodeTransform, Label: "Rescue", Config: raw(`{"template":"fetch failed but the run survived: {{fetch.output.error}}"}`), X: 160, Y: 340},
				},
				Edges: []Edge{
					{From: "start", To: "fetch"},
					{From: "fetch", To: "shape"},
					{From: "fetch", To: "rescue", Port: "error"},
				},
			},
		},
		{
			Name:        "team-router",
			Title:       "Team router (switch + merge)",
			Description: "Route a request to the right team's branch with a switch, then join the branches with a merge and summarize whichever ran.",
			Category:    "ops",
			Workflow: Workflow{
				Name:        "team-router",
				Description: "switch on payload.team → per-team transform → merge any → llm",
				Nodes: []Node{
					{ID: "start", Type: NodeTrigger, Label: "Manual", Config: raw(`{"kind":"manual"}`), X: 40, Y: 40},
					{ID: "route", Type: NodeSwitch, Label: "Which team?", Config: raw(`{"value":"{{trigger.payload.team}}","cases":[{"equals":"ops","port":"ops"},{"equals":"dev","port":"dev"}]}`), X: 40, Y: 190},
					{ID: "for_ops", Type: NodeTransform, Label: "Ops branch", Config: raw(`{"template":"OPS handles: {{trigger.payload.request}}"}`), X: -160, Y: 340},
					{ID: "for_dev", Type: NodeTransform, Label: "Dev branch", Config: raw(`{"template":"DEV handles: {{trigger.payload.request}}"}`), X: 40, Y: 340},
					{ID: "fallback", Type: NodeTransform, Label: "Unrouted", Config: raw(`{"template":"no team matched {{trigger.payload.team}}"}`), X: 240, Y: 340},
					{ID: "join", Type: NodeMerge, Label: "First branch wins", Config: raw(`{"mode":"any"}`), X: 40, Y: 490},
					{ID: "brief", Type: NodeLLM, Label: "Summarize", Config: raw(`{"prompt":"Summarize the routing outcome in one line: {{join.output}}"}`), X: 40, Y: 640},
				},
				Edges: []Edge{
					{From: "start", To: "route"},
					{From: "route", To: "for_ops", Port: "ops"},
					{From: "route", To: "for_dev", Port: "dev"},
					{From: "route", To: "fallback", Port: "default"},
					{From: "for_ops", To: "join"},
					{From: "for_dev", To: "join"},
					{From: "fallback", To: "join"},
					{From: "join", To: "brief"},
				},
			},
		},
		{
			Name:        "list-pipeline",
			Title:       "List pipeline (filter + map)",
			Description: "Filter a list from the start payload, template each surviving item, then remember the digest in memory — loops over data without loops.",
			Category:    "data",
			Workflow: Workflow{
				Name:        "list-pipeline",
				Description: "filter payload.items → map → memory remember",
				Nodes: []Node{
					{ID: "start", Type: NodeTrigger, Label: "Manual", Config: raw(`{"kind":"manual"}`), X: 40, Y: 40},
					{ID: "keep", Type: NodeFilter, Label: "score > 5", Config: raw(`{"items":"{{trigger.payload.items}}","left":"{{item.score}}","op":"gt","right":"5"}`), X: 40, Y: 190},
					{ID: "shape", Type: NodeMap, Label: "Per-item line", Config: raw(`{"items":"{{keep.output}}","template":"#{{index}} {{item.name}} ({{item.score}})"}`), X: 40, Y: 340},
					{ID: "save", Type: NodeTool, Label: "Remember digest", Config: raw(`{"tool":"memory","args":{"action":"remember","subject":"list-pipeline","content":"digest: {{shape.output}}"}}`), X: 40, Y: 490},
				},
				Edges: []Edge{
					{From: "start", To: "keep"},
					{From: "keep", To: "shape"},
					{From: "shape", To: "save"},
				},
			},
		},
	}
}

// TemplateByName returns the gallery entry with the given slug.
func TemplateByName(name string) (Template, bool) {
	for _, t := range Templates() {
		if t.Name == name {
			return t, true
		}
	}
	return Template{}, false
}
