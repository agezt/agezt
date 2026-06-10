// SPDX-License-Identifier: MIT

package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

// Manager wraps a Store with the kernel bus so every mutation is journaled
// (durable-before-publish) and carries the originating run's correlation_id.
// This is the journaling boundary — the Store itself stays pure. It mirrors
// how kernel/runtime publishes kernel.halt/resume rather than the state store
// doing its own journaling.
type Manager struct {
	store Store
	bus   *bus.Bus
	// now is the clock, injectable for deterministic tests. Defaults to
	// time.Now when constructed via NewManager.
	now func() time.Time
	// mu serialises the read-modify-write mutators (Remember/Forget/Supersede) so
	// two concurrent writers — e.g. the agent loop and the auto-distiller both
	// remembering a fact, or a reinforce racing a forget — cannot interleave their
	// Get→Put pairs and lose an update (M421). The underlying Store guards each call
	// individually but not the pair.
	mu sync.Mutex
}

// NewManager wires a Store to a bus. bus may be nil in tests that only
// exercise the store-facing behaviour, but production callers always pass the
// kernel bus so mutations are auditable.
func NewManager(store Store, b *bus.Bus) *Manager {
	return &Manager{store: store, bus: b, now: time.Now}
}

// RememberSpec is the input to Remember.
type RememberSpec struct {
	Type       Type
	Subject    string
	Content    string
	Tags       map[string]string
	Confidence float64
}

// Remember stores (or reinforces) a memory record and journals the write.
// Content-addressing means an identical (type, subject, content) triple
// dedupes onto the existing record: rather than creating a duplicate, the
// existing record's recency is refreshed and its confidence nudged up
// ("re-observed"). A previously tombstoned record is revived by a fresh
// Remember. Returns the record and whether it was newly created.
func (m *Manager) Remember(corr string, spec RememberSpec) (Record, bool, error) {
	if strings.TrimSpace(spec.Content) == "" {
		return Record{}, false, ErrEmptyContent
	}
	t := spec.Type
	if t == "" {
		t = DefaultType
	}
	if !ValidType(t) {
		return Record{}, false, fmt.Errorf("memory: invalid type %q", t)
	}
	conf := spec.Confidence
	if conf <= 0 {
		conf = 1.0
	}
	if conf > 1 {
		conf = 1
	}
	nowMS := m.now().UnixMilli()
	id := ContentID(t, spec.Subject, spec.Content)

	// Hold the lock across the Get→Put so a concurrent writer can't lose the update.
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, found, err := m.store.Get(id)
	if err != nil {
		return Record{}, false, err
	}

	rec := Record{
		ID:         id,
		Type:       t,
		Subject:    spec.Subject,
		Content:    spec.Content,
		Tags:       spec.Tags,
		Confidence: conf,
		CreatedMS:  nowMS,
		LastSeenMS: nowMS,
	}
	action := "create"
	if found {
		// Reinforce: keep original creation time, refresh recency, and
		// strengthen confidence toward 1.0. Revive if it was tombstoned.
		rec.CreatedMS = existing.CreatedMS
		rec.SourceEvent = existing.SourceEvent
		rec.Confidence = clampConf(existing.Confidence + 0.1)
		if existing.Tags != nil && rec.Tags == nil {
			rec.Tags = existing.Tags
		}
		// Preserve a supersession link: re-stating content that was explicitly
		// superseded must NOT silently resurrect it as active (it has a designated
		// successor). Without this, rec.SupersededBy="" would overwrite the link and
		// Active()/Recall would return the stale fact alongside its replacement.
		rec.SupersededBy = existing.SupersededBy
		action = "reinforce"
		if existing.Tombstoned {
			action = "revive"
		}
	}

	// Publish first so the event id can be recorded as provenance, then
	// persist. A crash between the two leaves an orphan audit event and no
	// stored record — harmless, since the store is the retrieval source of
	// truth and the journal is append-only audit.
	ev := m.publish(event.KindMemoryWritten, corr, map[string]any{
		"action":     action,
		"id":         id,
		"type":       string(t),
		"subject":    spec.Subject,
		"chars":      len(spec.Content),
		"confidence": rec.Confidence,
	})
	if ev != nil && rec.SourceEvent == "" {
		rec.SourceEvent = ev.ID
	}
	if err := m.store.Put(rec); err != nil {
		return Record{}, false, err
	}
	return rec, !found, nil
}

// Recall ranks active records against query and journals a memory.retrieved
// event (under corr) when anything matched, so `agt why` shows exactly what
// knowledge was surfaced for a task. Returns the ranked results (possibly
// empty).
func (m *Manager) Recall(corr, query string, limit int) ([]Scored, error) {
	return m.RecallScoped(corr, query, limit, "")
}

// RecallScoped is Recall restricted to a caller's visibility: shared records
// (no scope tag) are ALWAYS surfaced; a record private to some scope is surfaced
// only when that same scope is requested. An empty scope therefore sees shared
// memory only — which is what the daemon's automatic pre-run recall uses, so a
// run never inherits another agent's private notes (M652). This is the per-agent
// layer over the one shared brain: agents share most knowledge but can keep some
// notes to themselves by naming a scope.
func (m *Manager) RecallScoped(corr, query string, limit int, scope string) ([]Scored, error) {
	all, err := m.store.All()
	if err != nil {
		return nil, err
	}
	all = filterScope(all, scope)
	// Hybrid (M803): exact-keyword precision + local-embedding cosine for
	// typo/morphology recall — a misspelled or inflected query still
	// surfaces the right record.
	hits := SearchHybrid(all, query, limit, m.now().UnixMilli())
	if len(hits) > 0 {
		ids := make([]string, 0, len(hits))
		for _, h := range hits {
			ids = append(ids, h.Record.ID)
		}
		m.publish(event.KindMemoryRetrieved, corr, map[string]any{
			"query":   query,
			"matched": len(hits),
			"ids":     ids,
		})
	}
	return hits, nil
}

// Forget soft-deletes a record (tombstone) and journals it. The record stays
// on disk and in the journal — recall excludes it, but it can be recovered
// and the action is auditable/reversible. Returns false if id is unknown.
func (m *Manager) Forget(corr, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, found, err := m.store.Get(id)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if rec.Tombstoned {
		return true, nil // already forgotten; idempotent
	}
	rec.Tombstoned = true
	rec.LastSeenMS = m.now().UnixMilli()
	if err := m.store.Put(rec); err != nil {
		return false, err
	}
	m.publish(event.KindMemoryForgotten, corr, map[string]any{
		"id":      id,
		"subject": rec.Subject,
	})
	return true, nil
}

// Supersede replaces an existing record with a new one, linking the old
// record's SupersededBy to the new id (soft update — the old record is
// retained). Returns the new record. If oldID is unknown, the new record is
// still created (supersession of nothing is just a create).
func (m *Manager) Supersede(corr, oldID string, spec RememberSpec) (Record, error) {
	newRec, _, err := m.Remember(corr, spec)
	if err != nil {
		return Record{}, err
	}
	// Remember already released the lock; re-take it for the old-record Get→Put.
	// Distinct critical section (different id), so no re-entrancy.
	m.mu.Lock()
	defer m.mu.Unlock()
	old, found, err := m.store.Get(oldID)
	if err != nil {
		return Record{}, err
	}
	if found && old.ID != newRec.ID {
		old.SupersededBy = newRec.ID
		old.LastSeenMS = m.now().UnixMilli()
		if err := m.store.Put(old); err != nil {
			return Record{}, err
		}
		m.publish(event.KindMemorySuperseded, corr, map[string]any{
			"old_id": oldID,
			"new_id": newRec.ID,
		})
	}
	return newRec, nil
}

// Get returns a single record by id (any state). Used by the control plane's
// `agt memory get`.
func (m *Manager) Get(id string) (Record, bool, error) { return m.store.Get(id) }

// Active returns every non-tombstoned, non-superseded record, sorted
// deterministically. Used by `agt memory list` and as the recall corpus.
func (m *Manager) Active() ([]Record, error) {
	all, err := m.store.All()
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, r := range all {
		if r.Active() {
			out = append(out, r)
		}
	}
	return out, nil
}

// All returns every record including tombstoned/superseded ones.
func (m *Manager) All() ([]Record, error) { return m.store.All() }

// Count returns the total number of stored records (all states). Used by
// `agt status`.
func (m *Manager) Count() int { return m.store.Count() }

// Search ranks active records against query without journaling — used by the
// control plane's `agt memory search`, which is a read operation an operator
// runs ad hoc (Recall is the run-time path that journals provenance).
func (m *Manager) Search(query string, limit int) ([]Scored, error) {
	all, err := m.store.All()
	if err != nil {
		return nil, err
	}
	return SearchHybrid(all, query, limit, m.now().UnixMilli()), nil
}

// publish writes one event through the bus, returning the persisted event (or
// nil when no bus is wired, e.g. store-only tests). Subject groups memory
// events under "memory.<suffix>" so subscribers can scope-filter.
func (m *Manager) publish(kind event.Kind, corr string, payload any) *event.Event {
	if m.bus == nil {
		return nil
	}
	suffix := strings.TrimPrefix(string(kind), "memory.")
	ev, _ := m.bus.Publish(event.Spec{
		Subject:       "memory." + suffix,
		Kind:          kind,
		Actor:         "memory",
		CorrelationID: corr,
		Payload:       payload,
	})
	return ev
}

func clampConf(c float64) float64 {
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}

// --- run-time context plumbing -------------------------------------------

type ctxKey int

const ctxKeyCorrelation ctxKey = iota

// WithCorrelation returns a child context carrying corr so the in-process
// memory Tool can journal its writes under the originating run. The runtime
// sets this on every run's context; without it the tool falls back to an
// empty correlation (still journaled, just not linked).
func WithCorrelation(ctx context.Context, corr string) context.Context {
	return context.WithValue(ctx, ctxKeyCorrelation, corr)
}

// CorrelationFrom extracts the correlation id set by WithCorrelation.
func CorrelationFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
}

const ctxKeyScope ctxKey = iota + 1

// WithScope returns a child context carrying the run's per-agent memory scope
// (M786): when a run executes AS a named agent (M783), its recalls — the
// context injection and the memory tool — default to this scope, so the agent
// sees its own private notes on top of shared memory without having to name
// itself. Writes stay shared by default ("shared brain, private notes", M652);
// the explicit tool scope param always wins over this default.
func WithScope(ctx context.Context, scope string) context.Context {
	if scope == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyScope, scope)
}

// ScopeFrom extracts the per-agent memory scope set by WithScope ("" = none).
func ScopeFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyScope).(string); ok {
		return v
	}
	return ""
}

// --- agent tool -----------------------------------------------------------

// toolInputSchema is the JSON Schema advertised to the model for the
// in-process `memory` tool.
const toolInputSchema = `{
  "type": "object",
  "properties": {
    "action":  {"type": "string", "enum": ["remember", "recall", "forget"], "description": "what to do"},
    "subject": {"type": "string", "description": "entity/topic the memory is about (remember)"},
    "content": {"type": "string", "description": "the text to remember (remember)"},
    "type":    {"type": "string", "enum": ["FACT","SUMMARY","RELATION","PREFERENCE","OBSERVATION"], "description": "memory type (remember; default FACT)"},
    "query":   {"type": "string", "description": "search text (recall)"},
    "limit":   {"type": "integer", "description": "max results (recall; default 5)"},
    "id":      {"type": "string", "description": "record id (forget)"},
    "scope":   {"type": "string", "description": "optional private namespace, e.g. your role like \"researcher\". On remember: keep this note private to that scope. On recall: also surface that scope's private notes. Shared memory (no scope) is ALWAYS visible; another scope's private notes never are. Omit for shared memory."}
  },
  "required": ["action"]
}`

type toolInput struct {
	Action  string `json:"action"`
	Subject string `json:"subject"`
	Content string `json:"content"`
	Type    Type   `json:"type"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
	ID      string `json:"id"`
	Scope   string `json:"scope"`
}

// memoryTool is the in-process agent.Tool that lets the agent remember,
// recall, and forget during a run. Writes are journaled by the Manager under
// the run's correlation (read from ctx via CorrelationFrom).
type memoryTool struct{ mgr *Manager }

// Tool returns the agent-facing memory tool. Register it under the name
// "memory" in the agent loop's tool map.
func (m *Manager) Tool() agent.Tool { return memoryTool{mgr: m} }

func (t memoryTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "memory",
		Description: "Persist and retrieve durable knowledge across tasks. " +
			"action=remember stores a fact (subject, content); " +
			"action=recall searches stored memory (query); " +
			"action=forget tombstones a record (id). " +
			"Memory is shared with every agent by default; pass an optional scope " +
			"(e.g. your role) to keep a note private to that scope and to recall it.",
		InputSchema: json.RawMessage(toolInputSchema),
	}
}

func (t memoryTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid memory input: " + err.Error(), IsError: true}, nil
	}
	corr := CorrelationFrom(ctx)
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "remember":
		rec, created, err := t.mgr.Remember(corr, RememberSpec{
			Type: in.Type, Subject: in.Subject, Content: in.Content, Tags: in.Tags(),
		})
		if err != nil {
			return agent.Result{Output: "remember failed: " + err.Error(), IsError: true}, nil
		}
		verb := "reinforced"
		if created {
			verb = "stored"
		}
		return agent.Result{Output: fmt.Sprintf("%s memory %s (%s: %s)", verb, rec.ID[:12], rec.Type, rec.Subject)}, nil
	case "recall":
		limit := in.Limit
		if limit <= 0 {
			limit = 5
		}
		// The run's per-agent scope (M786) is the DEFAULT visibility: a named
		// agent recalls its own private notes + shared memory without naming
		// itself. An explicit scope param wins (e.g. peeking at a teammate's
		// scope is still expressible — records stay readable, never hidden
		// behind identity).
		scope := strings.TrimSpace(in.Scope)
		if scope == "" {
			scope = ScopeFrom(ctx)
		}
		hits, err := t.mgr.RecallScoped(corr, in.Query, limit, scope)
		if err != nil {
			return agent.Result{Output: "recall failed: " + err.Error(), IsError: true}, nil
		}
		return agent.Result{Output: renderHits(hits)}, nil
	case "forget":
		ok, err := t.mgr.Forget(corr, in.ID)
		if err != nil {
			return agent.Result{Output: "forget failed: " + err.Error(), IsError: true}, nil
		}
		if !ok {
			return agent.Result{Output: "no memory with id " + in.ID, IsError: true}, nil
		}
		return agent.Result{Output: "forgot memory " + in.ID}, nil
	default:
		return agent.Result{Output: "unknown action " + in.Action + " (remember|recall|forget)", IsError: true}, nil
	}
}

// Tags adapts the tool input to a tag map. Tool writes are tagged source=agent so
// they are distinguishable from operator and distilled writes; an optional scope
// tag makes the note private to that namespace (recall only surfaces it when the
// same scope is requested) — the per-agent layer over shared memory (M652).
func (in toolInput) Tags() map[string]string {
	t := map[string]string{"source": "agent"}
	if s := strings.TrimSpace(in.Scope); s != "" {
		t["scope"] = s
	}
	return t
}

// filterScope drops records private to a scope other than the requested one.
// A record is visible when it carries no scope tag (shared) or its scope equals
// the caller's. Returns a new slice; the input is not mutated.
func filterScope(recs []Record, scope string) []Record {
	out := make([]Record, 0, len(recs))
	for _, r := range recs {
		rs := ""
		if r.Tags != nil {
			rs = r.Tags["scope"]
		}
		if rs == "" || rs == scope {
			out = append(out, r)
		}
	}
	return out
}

func renderHits(hits []Scored) string {
	if len(hits) == 0 {
		return "no relevant memory found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d relevant memor%s:\n", len(hits), plural(len(hits)))
	for _, h := range hits {
		scope := ""
		if h.Record.Tags != nil && h.Record.Tags["scope"] != "" {
			scope = " (scope: " + h.Record.Tags["scope"] + ")"
		}
		fmt.Fprintf(&b, "- [%s] %s: %s%s\n", h.Record.Type, h.Record.Subject, h.Record.Content, scope)
	}
	return strings.TrimRight(b.String(), "\n")
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// --- distillation ---------------------------------------------------------

// distillSystem instructs the provider to extract durable, reusable facts
// from a completed task. The model must return a JSON object so parsing is
// deterministic; any non-JSON or empty response yields zero facts (the
// best-effort contract — distillation never fails a task).
const distillSystem = `You review a completed agent task and extract durable, reusable facts worth remembering for future tasks. ` +
	`Return ONLY a JSON object of the form {"facts":[{"subject":"...","content":"...","type":"FACT|SUMMARY|PREFERENCE"}]}. ` +
	`Extract at most 3 facts. Prefer specific, durable knowledge (project structure, decisions, user preferences) over transient details. ` +
	`If nothing is worth remembering, return {"facts":[]}.`

type distillResult struct {
	Facts []struct {
		Subject string `json:"subject"`
		Content string `json:"content"`
		Type    Type   `json:"type"`
	} `json:"facts"`
}

// Distill runs one best-effort LLM call over a task transcript and stores any
// extracted facts (tagged source=distill) under corr. It returns the ids it
// created. Errors are returned for the caller to journal, but the caller must
// never let a distillation error fail the underlying task.
func (m *Manager) Distill(ctx context.Context, corr string, provider agent.Provider, model, intent, transcript string) ([]string, error) {
	if provider == nil {
		return nil, errors.New("memory: distill requires a provider")
	}
	user := fmt.Sprintf("Task intent:\n%s\n\nWhat happened:\n%s", intent, transcript)
	resp, err := provider.Complete(ctx, agent.CompletionRequest{
		Model:    model,
		System:   distillSystem,
		Messages: []agent.Message{{Role: agent.RoleUser, Content: user}},
		TaskType: "distill",
	})
	if err != nil {
		return nil, fmt.Errorf("memory: distill completion: %w", err)
	}
	parsed, ok := parseDistill(resp.Message.Content)
	if !ok {
		// Non-JSON answer (e.g. the mock provider) → nothing to store. Not
		// an error; distillation is opportunistic.
		return nil, nil
	}
	var ids []string
	for _, f := range parsed.Facts {
		if strings.TrimSpace(f.Content) == "" {
			continue
		}
		t := f.Type
		if !ValidType(t) {
			t = TypeSummary
		}
		rec, _, err := m.Remember(corr, RememberSpec{
			Type:    t,
			Subject: f.Subject,
			Content: f.Content,
			Tags:    map[string]string{"source": "distill"},
		})
		if err != nil {
			return ids, err
		}
		ids = append(ids, rec.ID)
	}
	return ids, nil
}

// parseDistill extracts the JSON object from a model response, tolerating
// surrounding prose or markdown fences by scanning for the outermost braces.
func parseDistill(s string) (distillResult, bool) {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return distillResult{}, false
	}
	var r distillResult
	if err := json.Unmarshal([]byte(s[start:end+1]), &r); err != nil {
		return distillResult{}, false
	}
	return r, true
}
