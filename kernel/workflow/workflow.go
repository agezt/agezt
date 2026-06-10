// SPDX-License-Identifier: MIT

// Package workflow is the n8n-style workflow engine (M798): durable, named
// graphs of TYPED nodes — trigger, tool, llm, condition, transform, delay —
// wired by edges and carrying data between nodes with {{path}} templates.
// Unlike kernel/planner (intent-in, agent-loop-per-node), a workflow node is
// a precise, deterministic step: THIS tool with THESE args, THIS prompt to
// THIS model, THIS branch on THIS value. The graph is what you see on the
// console canvas; the engine (kernel/runtime) executes it under the same
// governance as everything else — tool nodes pass Edict, llm nodes ride the
// Governor, every step is journaled (workflow.*).
//
// Storage mirrors kernel/standing: one atomic JSON file, journaled CRUD.
package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/ulid"
)

// ErrNotFound is returned for an unknown workflow id/name.
var ErrNotFound = errors.New("workflow: not found")

// Node types the engine executes.
const (
	NodeTrigger   = "trigger"   // the single entry point: manual | cron | event (M799)
	NodeTool      = "tool"      // one governed tool call: config {tool, args}
	NodeLLM       = "llm"       // one completion: config {prompt, system?, model?}
	NodeCondition = "condition" // branch: config {left, op, right} → "true"/"false" ports
	NodeTransform = "transform" // pure template: config {template} → output
	NodeDelay     = "delay"     // wait: config {seconds}

	// The M800 library.
	NodeHTTP     = "http"        // request via the governed http tool: {method, url, headers?, body?}
	NodeCode     = "code"        // sandboxed script: {language, code, input?} (input → stdin.txt)
	NodeMap      = "map"         // per-item template over an array: {items, template} ({{item}}, {{index}})
	NodeFilter   = "filter"      // keep matching items: {items, left, op, right} ({{item}} in left/right)
	NodeSwitch   = "switch"      // multi-way branch: {value, cases:[{equals, port}]} → case ports | "default"
	NodeMerge    = "merge"       // join branches: {mode: "all"|"any"} (all = wait for every incoming edge)
	NodeApproval = "approval"    // HITL gate: {description, capability?} — blocks on the operator
	NodeSubflow  = "subworkflow" // run another stored workflow: {workflow, payload?} (depth-capped)
)

// knownTypes gates validation; ordered for error messages.
var knownTypes = []string{
	NodeTrigger, NodeTool, NodeLLM, NodeCondition, NodeTransform, NodeDelay,
	NodeHTTP, NodeCode, NodeMap, NodeFilter, NodeSwitch, NodeMerge, NodeApproval, NodeSubflow,
}

// Failable nodes may wire an "error" port: when the node fails AND such an
// edge exists, the run survives — {{node.output.error}} carries the message
// and the error branch runs instead of the default one.
var failable = map[string]bool{
	NodeTool: true, NodeLLM: true, NodeHTTP: true, NodeCode: true,
	NodeApproval: true, NodeSubflow: true,
}

// Node is one typed step on the canvas.
type Node struct {
	ID    string `json:"id"`   // unique within the workflow, [a-z0-9_-]
	Type  string `json:"type"` // one of the Node* constants
	Label string `json:"label,omitempty"`
	// Config is the type-specific payload (see the constants above). Its
	// string fields may carry {{path}} templates resolved at run time
	// against upstream outputs: {{trigger.payload}}, {{<node_id>.output}}.
	Config json.RawMessage `json:"config,omitempty"`
	// X/Y are canvas coordinates — pure presentation, never semantics.
	X float64 `json:"x,omitempty"`
	Y float64 `json:"y,omitempty"`

	// Reliability settings (M808) — production runs need per-node control,
	// not per-workflow hope. TimeoutSec bounds ONE attempt (0 = no extra
	// bound beyond the run's own deadline). Retries re-runs a FAILABLE node
	// after a failure (the error port only fires once retries are
	// exhausted); RetryDelaySec pauses between attempts.
	TimeoutSec    int `json:"timeout_sec,omitempty"`
	Retries       int `json:"retries,omitempty"`
	RetryDelaySec int `json:"retry_delay_sec,omitempty"`
}

// Edge wires From's completion to To's execution. Port selects a labelled
// output: a condition node fires exactly one of "true"/"false"; every other
// node fires its "" (default) port.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Port string `json:"port,omitempty"`
}

// Workflow is one durable, named graph.
type Workflow struct {
	ID   string `json:"id"`
	Name string `json:"name"` // unique handle, immutable shape rules like roster slugs
	// Enabled gates triggers (M799) — a disabled workflow never auto-fires;
	// a manual run is still allowed (that's how you test a draft).
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
	Nodes       []Node `json:"nodes"`
	Edges       []Edge `json:"edges,omitempty"`
	CreatedMS   int64  `json:"created_ms"`
	UpdatedMS   int64  `json:"updated_ms"`
}

var (
	nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	idRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
)

const (
	maxNodes        = 100
	maxEdges        = 300
	maxConfigBytes  = 32 * 1024
	maxDelaySeconds = 600
)

// Validate checks a workflow's shape: name/id rules, exactly one trigger,
// known node types, edges that resolve, legal ports, per-type config, and an
// acyclic graph. The engine refuses to run anything Validate rejects.
func Validate(w Workflow) error {
	if !nameRe.MatchString(w.Name) {
		return fmt.Errorf("workflow: name must match %s", nameRe)
	}
	if len(w.Nodes) == 0 {
		return errors.New("workflow: at least one node (the trigger) is required")
	}
	if len(w.Nodes) > maxNodes {
		return fmt.Errorf("workflow: at most %d nodes", maxNodes)
	}
	if len(w.Edges) > maxEdges {
		return fmt.Errorf("workflow: at most %d edges", maxEdges)
	}

	byID := make(map[string]*Node, len(w.Nodes))
	triggers := 0
	for i := range w.Nodes {
		n := &w.Nodes[i]
		if !idRe.MatchString(n.ID) {
			return fmt.Errorf("workflow: node id %q must match %s", n.ID, idRe)
		}
		if _, dup := byID[n.ID]; dup {
			return fmt.Errorf("workflow: duplicate node id %q", n.ID)
		}
		byID[n.ID] = n
		if len(n.Config) > maxConfigBytes {
			return fmt.Errorf("workflow: node %s config exceeds %d bytes", n.ID, maxConfigBytes)
		}
		if err := validateNodeConfig(n); err != nil {
			return err
		}
		if n.Type == NodeTrigger {
			triggers++
		}
	}
	if triggers != 1 {
		return fmt.Errorf("workflow: exactly one trigger node required, found %d", triggers)
	}

	indeg := map[string]int{}
	adj := map[string][]string{}
	for _, e := range w.Edges {
		from, ok := byID[e.From]
		if !ok {
			return fmt.Errorf("workflow: edge from unknown node %q", e.From)
		}
		if _, ok := byID[e.To]; !ok {
			return fmt.Errorf("workflow: edge to unknown node %q", e.To)
		}
		if e.From == e.To {
			return fmt.Errorf("workflow: self-edge on %q", e.From)
		}
		if err := validateEdgePort(from, e.Port); err != nil {
			return err
		}
		if from.Type == NodeTrigger && byID[e.To].Type == NodeTrigger {
			return errors.New("workflow: trigger cannot feed a trigger")
		}
		adj[e.From] = append(adj[e.From], e.To)
		indeg[e.To]++
	}
	for _, n := range w.Nodes {
		if n.Type == NodeTrigger && indeg[n.ID] > 0 {
			return errors.New("workflow: the trigger cannot have incoming edges")
		}
	}

	// Acyclicity (Kahn) over the whole graph.
	queue := make([]string, 0, len(w.Nodes))
	deg := make(map[string]int, len(w.Nodes))
	for id := range byID {
		deg[id] = indeg[id]
		if indeg[id] == 0 {
			queue = append(queue, id)
		}
	}
	seen := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		seen++
		for _, next := range adj[id] {
			deg[next]--
			if deg[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if seen != len(w.Nodes) {
		return errors.New("workflow: the graph has a cycle")
	}
	return nil
}

// TriggerConfig is the trigger node's config (M799): how a workflow STARTS.
// kind "" or "manual" = run-on-demand only. "cron" fires on a clock —
// either every interval_sec seconds (≥30) or once a day at daily_at
// ("HH:MM", daemon-local time). "event" fires when a journal event's
// subject matches the glob (bus semantics: "*" = one token, ">" = rest),
// with the event riding in as {{trigger.payload}}. Triggers only arm while
// the workflow is ENABLED.
type TriggerConfig struct {
	Kind        string `json:"kind,omitempty"`
	IntervalSec int    `json:"interval_sec,omitempty"` // cron: every N seconds (≥30)
	DailyAt     string `json:"daily_at,omitempty"`     // cron: "HH:MM" once a day
	Subject     string `json:"subject,omitempty"`      // event: subject glob
	// Secret authenticates the webhook kind (M809): an external POST to
	// /hooks/<name> must present it. Per-workflow, never the console token —
	// a leaked hook secret can fire ONE workflow, nothing else.
	Secret string `json:"secret,omitempty"`
	// Reply (M812) makes the webhook SYNCHRONOUS: the caller's POST holds
	// until the run finishes and the response carries the outputs — n8n's
	// "respond to webhook". Reply workflows should be fast (the sync path
	// is capped well under the async 15m); long pipelines stay async.
	Reply bool `json:"reply,omitempty"`
}

const (
	minIntervalSec      = 30
	minWebhookSecretLen = 12
)

var dailyAtRe = regexp.MustCompile(`^([01]?\d|2[0-3]):[0-5]\d$`)

// TriggerSpec parses a workflow's trigger configuration (Validate
// guarantees it parses and is legal).
func (w Workflow) TriggerSpec() TriggerConfig {
	var c TriggerConfig
	if n := w.TriggerNode(); n != nil {
		_ = json.Unmarshal(orEmpty(n.Config), &c)
	}
	if c.Kind == "" {
		c.Kind = "manual"
	}
	return c
}

func validateTriggerConfig(n *Node) error {
	var c TriggerConfig
	if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
		return fmt.Errorf("workflow: node %s: trigger config: %w", n.ID, err)
	}
	switch c.Kind {
	case "", "manual":
		return nil
	case "cron":
		hasInterval := c.IntervalSec != 0
		hasDaily := strings.TrimSpace(c.DailyAt) != ""
		if hasInterval == hasDaily { // both or neither
			return fmt.Errorf("workflow: node %s: cron trigger needs exactly one of interval_sec or daily_at", n.ID)
		}
		if hasInterval && c.IntervalSec < minIntervalSec {
			return fmt.Errorf("workflow: node %s: interval_sec must be >= %d", n.ID, minIntervalSec)
		}
		if hasDaily && !dailyAtRe.MatchString(strings.TrimSpace(c.DailyAt)) {
			return fmt.Errorf("workflow: node %s: daily_at must be HH:MM", n.ID)
		}
		return nil
	case "event":
		subj := strings.TrimSpace(c.Subject)
		if subj == "" {
			return fmt.Errorf("workflow: node %s: event trigger needs a subject glob", n.ID)
		}
		if strings.HasPrefix(subj, "workflow.") || subj == ">" || subj == "*" {
			// A workflow run publishes workflow.* events — triggering on them
			// (or on everything) is a feedback-loop foot-gun, refused outright.
			return fmt.Errorf("workflow: node %s: event subject %q is too broad or self-referential", n.ID, subj)
		}
		return nil
	case "webhook":
		// The secret is the ONLY gate between the internet-facing hook path
		// and a workflow run — a short one is a refused config, not a risk
		// the operator silently accepts.
		if len(strings.TrimSpace(c.Secret)) < minWebhookSecretLen {
			return fmt.Errorf("workflow: node %s: webhook trigger needs a secret of at least %d characters", n.ID, minWebhookSecretLen)
		}
		return nil
	default:
		return fmt.Errorf("workflow: node %s: unknown trigger kind %q (manual|cron|event|webhook)", n.ID, c.Kind)
	}
}

// Per-type config shapes (also the engine's parse targets).
type ToolConfig struct {
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args,omitempty"` // templated JSON
}

type LLMConfig struct {
	Prompt string `json:"prompt"` // templated
	System string `json:"system,omitempty"`
	Model  string `json:"model,omitempty"`
}

type ConditionConfig struct {
	Left  string `json:"left"` // templated
	Op    string `json:"op"`   // equals|not_equals|contains|not_empty|empty|gt|lt
	Right string `json:"right,omitempty"`
}

type TransformConfig struct {
	Template string `json:"template"` // templated → becomes the node's output
}

type DelayConfig struct {
	Seconds float64 `json:"seconds"`
}

// HTTPConfig rides the registered `http` tool — same egress guard and
// CapHTTPGet/Post policy as an agent's own call. URL/headers/body are
// templated.
type HTTPConfig struct {
	Method      string            `json:"method"` // GET | POST
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
}

// CodeConfig runs a script in the code-exec sandbox (the M794 runner):
// the interpolated Input lands as ./stdin.txt, stdout becomes the output.
type CodeConfig struct {
	Language string `json:"language"`
	Code     string `json:"code"`
	Input    string `json:"input,omitempty"` // templated; default "{}"
}

// MapConfig applies Template to every element of the array at Items
// (a {{...}}-style path); inside the template {{item}} / {{item.field}} /
// {{index}} address the current element.
type MapConfig struct {
	Items    string `json:"items"`
	Template string `json:"template"`
}

// FilterConfig keeps the elements of Items for which left/op/right holds
// ({{item}} / {{index}} usable in left and right).
type FilterConfig struct {
	Items string `json:"items"`
	Left  string `json:"left"`
	Op    string `json:"op"`
	Right string `json:"right,omitempty"`
}

// SwitchConfig routes by value: the first case whose Equals matches fires
// its Port; otherwise the "default" port fires.
type SwitchCase struct {
	Equals string `json:"equals"`
	Port   string `json:"port"`
}

type SwitchConfig struct {
	Value string       `json:"value"` // templated
	Cases []SwitchCase `json:"cases"`
}

// MergeConfig joins branches: "any" (default) runs on the first incoming
// token; "all" waits for a token on EVERY incoming edge — if an upstream
// branch never fires (an untaken condition path), the merge never runs.
type MergeConfig struct {
	Mode string `json:"mode,omitempty"`
}

// ApprovalConfig blocks the run on a human decision via the HITL approval
// registry (`agt approvals` / channels / console). Deny or timeout fails
// the node (wire an "error" port to branch instead).
type ApprovalConfig struct {
	Description string `json:"description"` // templated; what the operator reads
	Capability  string `json:"capability,omitempty"`
}

// SubflowConfig runs another stored workflow; the interpolated Payload
// becomes its {{trigger.payload}}. Nesting is depth-capped by the engine.
type SubflowConfig struct {
	Workflow string `json:"workflow"`
	Payload  string `json:"payload,omitempty"` // templated; JSON becomes structured
}

const maxSwitchCases = 16

var conditionOps = map[string]bool{
	"equals": true, "not_equals": true, "contains": true,
	"not_empty": true, "empty": true, "gt": true, "lt": true,
}

// validateEdgePort enforces each node type's legal output ports: condition
// fires true/false, switch fires its declared case ports or "default",
// failable nodes may add an "error" branch, everything else uses the
// default port only.
func validateEdgePort(from *Node, port string) error {
	switch from.Type {
	case NodeCondition:
		if port != "true" && port != "false" {
			return fmt.Errorf("workflow: edge from condition %q needs port \"true\" or \"false\"", from.ID)
		}
		return nil
	case NodeSwitch:
		if port == "default" {
			return nil
		}
		var c SwitchConfig
		_ = json.Unmarshal(orEmpty(from.Config), &c)
		for _, cs := range c.Cases {
			if cs.Port == port {
				return nil
			}
		}
		return fmt.Errorf("workflow: edge from switch %q uses undeclared port %q", from.ID, port)
	default:
		if port == "" {
			return nil
		}
		if port == "error" && failable[from.Type] {
			return nil
		}
		return fmt.Errorf("workflow: edge from %q (%s) must use the default port", from.ID, from.Type)
	}
}

// Reliability bounds (M808): a node may not retry forever or sleep the run
// away between attempts.
const (
	maxNodeTimeoutSec = 600
	maxNodeRetries    = 5
	maxRetryDelaySec  = 60
)

// validateReliability checks the per-node retry/timeout settings. Retries
// only make sense on failable nodes (a transform never fails transiently);
// timeouts apply to anything that does work, so everything but the trigger.
func validateReliability(n *Node) error {
	if n.TimeoutSec < 0 || n.TimeoutSec > maxNodeTimeoutSec {
		return fmt.Errorf("workflow: node %s: timeout_sec must be 0..%d", n.ID, maxNodeTimeoutSec)
	}
	if n.TimeoutSec > 0 && n.Type == NodeTrigger {
		return fmt.Errorf("workflow: node %s: the trigger does not run — timeout_sec is meaningless on it", n.ID)
	}
	if n.Retries < 0 || n.Retries > maxNodeRetries {
		return fmt.Errorf("workflow: node %s: retries must be 0..%d", n.ID, maxNodeRetries)
	}
	if n.RetryDelaySec < 0 || n.RetryDelaySec > maxRetryDelaySec {
		return fmt.Errorf("workflow: node %s: retry_delay_sec must be 0..%d", n.ID, maxRetryDelaySec)
	}
	if (n.Retries > 0 || n.RetryDelaySec > 0) && !failable[n.Type] {
		return fmt.Errorf("workflow: node %s: retries only apply to failable nodes (tool/llm/http/code/approval/subworkflow)", n.ID)
	}
	return nil
}

func validateNodeConfig(n *Node) error {
	if err := validateReliability(n); err != nil {
		return err
	}
	switch n.Type {
	case NodeTrigger:
		return validateTriggerConfig(n)
	case NodeTool:
		var c ToolConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: tool config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Tool) == "" {
			return fmt.Errorf("workflow: node %s: tool name is required", n.ID)
		}
		return nil
	case NodeLLM:
		var c LLMConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: llm config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Prompt) == "" {
			return fmt.Errorf("workflow: node %s: llm prompt is required", n.ID)
		}
		return nil
	case NodeCondition:
		var c ConditionConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: condition config: %w", n.ID, err)
		}
		if !conditionOps[c.Op] {
			return fmt.Errorf("workflow: node %s: condition op %q (want equals|not_equals|contains|not_empty|empty|gt|lt)", n.ID, c.Op)
		}
		return nil
	case NodeTransform:
		var c TransformConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: transform config: %w", n.ID, err)
		}
		if c.Template == "" {
			return fmt.Errorf("workflow: node %s: transform template is required", n.ID)
		}
		return nil
	case NodeDelay:
		var c DelayConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: delay config: %w", n.ID, err)
		}
		if c.Seconds <= 0 || c.Seconds > maxDelaySeconds {
			return fmt.Errorf("workflow: node %s: delay seconds must be in (0, %d]", n.ID, maxDelaySeconds)
		}
		return nil
	case NodeHTTP:
		var c HTTPConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: http config: %w", n.ID, err)
		}
		m := strings.ToUpper(strings.TrimSpace(c.Method))
		if m != "GET" && m != "POST" {
			return fmt.Errorf("workflow: node %s: http method must be GET or POST", n.ID)
		}
		if strings.TrimSpace(c.URL) == "" {
			return fmt.Errorf("workflow: node %s: http url is required", n.ID)
		}
		return nil
	case NodeCode:
		var c CodeConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: code config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Language) == "" || strings.TrimSpace(c.Code) == "" {
			return fmt.Errorf("workflow: node %s: code needs language and code", n.ID)
		}
		return nil
	case NodeMap:
		var c MapConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: map config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Items) == "" || c.Template == "" {
			return fmt.Errorf("workflow: node %s: map needs items and template", n.ID)
		}
		return nil
	case NodeFilter:
		var c FilterConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: filter config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Items) == "" {
			return fmt.Errorf("workflow: node %s: filter needs items", n.ID)
		}
		if !conditionOps[c.Op] {
			return fmt.Errorf("workflow: node %s: filter op %q (want equals|not_equals|contains|not_empty|empty|gt|lt)", n.ID, c.Op)
		}
		return nil
	case NodeSwitch:
		var c SwitchConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: switch config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Value) == "" {
			return fmt.Errorf("workflow: node %s: switch needs a value", n.ID)
		}
		if len(c.Cases) == 0 || len(c.Cases) > maxSwitchCases {
			return fmt.Errorf("workflow: node %s: switch needs 1..%d cases", n.ID, maxSwitchCases)
		}
		seen := map[string]bool{}
		for _, cs := range c.Cases {
			p := strings.TrimSpace(cs.Port)
			if p == "" || p == "default" || p == "error" {
				return fmt.Errorf("workflow: node %s: switch case ports must be named (not empty/default/error)", n.ID)
			}
			if seen[p] {
				return fmt.Errorf("workflow: node %s: duplicate switch port %q", n.ID, p)
			}
			seen[p] = true
		}
		return nil
	case NodeMerge:
		var c MergeConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: merge config: %w", n.ID, err)
		}
		if c.Mode != "" && c.Mode != "all" && c.Mode != "any" {
			return fmt.Errorf("workflow: node %s: merge mode must be \"all\" or \"any\"", n.ID)
		}
		return nil
	case NodeApproval:
		var c ApprovalConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: approval config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Description) == "" {
			return fmt.Errorf("workflow: node %s: approval needs a description (what the operator reads)", n.ID)
		}
		return nil
	case NodeSubflow:
		var c SubflowConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: subworkflow config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Workflow) == "" {
			return fmt.Errorf("workflow: node %s: subworkflow needs a workflow name", n.ID)
		}
		return nil
	default:
		return fmt.Errorf("workflow: node %s: unknown type %q (want %s)", n.ID, n.Type, strings.Join(knownTypes, "|"))
	}
}

func orEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

// TriggerNode returns the workflow's single trigger (Validate guarantees it).
func (w Workflow) TriggerNode() *Node {
	for i := range w.Nodes {
		if w.Nodes[i].Type == NodeTrigger {
			return &w.Nodes[i]
		}
	}
	return nil
}

// NodeByID resolves one node.
func (w Workflow) NodeByID(id string) *Node {
	for i := range w.Nodes {
		if w.Nodes[i].ID == id {
			return &w.Nodes[i]
		}
	}
	return nil
}

// Store is the persistent workflow registry, a single JSON file rewritten
// atomically on change. Safe for concurrent use. Mirrors kernel/standing.
type Store struct {
	path  string
	mu    sync.Mutex
	now   func() time.Time
	items []*Workflow
}

// OpenStore opens (or creates) the registry under dir.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("workflow: mkdir %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, "workflows.json"), now: time.Now}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("workflow: read %s: %w", s.path, err)
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.items); err != nil {
			return nil, fmt.Errorf("workflow: parse %s: %w", s.path, err)
		}
	}
	return s, nil
}

// Save upserts a workflow by name: a new name is created (id assigned,
// enabled by default), an existing one is replaced wholesale — the canvas
// always posts the complete graph. Identity/lifecycle fields (ID, CreatedMS,
// Enabled) survive an update. Returns the stored value + whether it was
// created.
func (s *Store) Save(w Workflow) (Workflow, bool, error) {
	w.Name = strings.TrimSpace(w.Name)
	if err := Validate(w); err != nil {
		return Workflow{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UnixMilli()
	for _, ex := range s.items {
		if ex.Name == w.Name {
			snapshot := *ex
			w.ID, w.CreatedMS, w.Enabled = ex.ID, ex.CreatedMS, ex.Enabled
			w.UpdatedMS = now
			*ex = w
			if err := s.save(); err != nil {
				*ex = snapshot
				return Workflow{}, false, err
			}
			return *ex, false, nil
		}
	}
	w.ID = ulid.New()
	w.Enabled = true
	w.CreatedMS = now
	w.UpdatedMS = now
	cp := w
	s.items = append(s.items, &cp)
	if err := s.save(); err != nil {
		s.items = s.items[:len(s.items)-1]
		return Workflow{}, false, err
	}
	return cp, true, nil
}

// SetEnabled flips trigger-arming for a workflow by id or name.
func (s *Store) SetEnabled(ref string, enabled bool) (Workflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.find(ref)
	if w == nil {
		return Workflow{}, ErrNotFound
	}
	prevEnabled, prevUpdated := w.Enabled, w.UpdatedMS
	w.Enabled = enabled
	w.UpdatedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		w.Enabled, w.UpdatedMS = prevEnabled, prevUpdated
		return Workflow{}, err
	}
	return *w, nil
}

// Remove deletes a workflow by id or name. Returns whether it existed.
func (s *Store) Remove(ref string) (Workflow, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, w := range s.items {
		if w.ID == ref || w.Name == ref {
			removed := s.items
			gone := *w
			s.items = append(append([]*Workflow{}, s.items[:i]...), s.items[i+1:]...)
			if err := s.save(); err != nil {
				s.items = removed
				return Workflow{}, false, err
			}
			return gone, true, nil
		}
	}
	return Workflow{}, false, nil
}

// Get returns one workflow by id or name.
func (s *Store) Get(ref string) (Workflow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w := s.find(ref); w != nil {
		return *w, true
	}
	return Workflow{}, false
}

func (s *Store) find(ref string) *Workflow {
	for _, w := range s.items {
		if w.ID == ref || w.Name == ref {
			return w
		}
	}
	return nil
}

// List returns all workflows, sorted by creation time then id.
func (s *Store) List() []Workflow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Workflow, 0, len(s.items))
	for _, w := range s.items {
		out = append(out, *w)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedMS != out[j].CreatedMS {
			return out[i].CreatedMS < out[j].CreatedMS
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Count returns the number of stored workflows.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
