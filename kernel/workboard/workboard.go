// SPDX-License-Identifier: MIT

// Package workboard is AGEZT's durable typed task queue. It is deliberately
// separate from kernel/board: board is a message mailbox, workboard is a
// restart-safe task state machine that agents and workflows can coordinate on.
package workboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/proof"
	"github.com/agezt/agezt/kernel/ulid"
)

const (
	storeVersion = 1
	maxTasks     = 10000
)

var (
	ErrNotFound      = errors.New("workboard: not found")
	ErrInvalidStatus = errors.New("workboard: invalid status")
	ErrClaimConflict = errors.New("workboard: claim conflict")
	ErrNotClaimed    = errors.New("workboard: not claimed")
	ErrClaimFresh    = errors.New("workboard: claim heartbeat is not stale")
	ErrInvalidPolicy = errors.New("workboard: invalid retry policy")
	ErrUnproven      = errors.New("workboard: task has unsatisfied acceptance criteria; prove it before completing")
)

// Status is the durable lifecycle state of a workboard task.
type Status string

const (
	StatusTriage   Status = "triage"
	StatusTodo     Status = "todo"
	StatusReady    Status = "ready"
	StatusRunning  Status = "running"
	StatusBlocked  Status = "blocked"
	StatusReview   Status = "review"
	StatusDone     Status = "done"
	StatusArchived Status = "archived"
)

var statusOrder = map[Status]int{
	StatusTriage:   0,
	StatusTodo:     1,
	StatusReady:    2,
	StatusRunning:  3,
	StatusBlocked:  4,
	StatusReview:   5,
	StatusDone:     6,
	StatusArchived: 7,
}

// ParseStatus normalizes and validates a status string.
func ParseStatus(s string) (Status, error) {
	st := Status(strings.ToLower(strings.TrimSpace(s)))
	if st == "" {
		return "", ErrInvalidStatus
	}
	if _, ok := statusOrder[st]; !ok {
		return "", fmt.Errorf("%w: %s", ErrInvalidStatus, s)
	}
	return st, nil
}

// Task is one durable work item. Tasks are not agents: an agent may own, claim,
// block, complete, link, or review the task, but the task remains a typed record.
type Task struct {
	ID             string            `json:"id"`
	Title          string            `json:"title"`
	Description    string            `json:"description,omitempty"`
	Status         Status            `json:"status"`
	Priority       int               `json:"priority,omitempty"`
	Tenant         string            `json:"tenant,omitempty"`
	Assignee       string            `json:"assignee,omitempty"`
	Owner          string            `json:"owner,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	Artifacts      []string          `json:"artifacts,omitempty"`
	Criteria       []proof.Criterion `json:"criteria,omitempty"`
	Proof          *proof.Proof      `json:"proof,omitempty"`
	Seat           string            `json:"seat,omitempty"`
	RetryPolicy    *RetryPolicy      `json:"retry_policy,omitempty"`
	Claim          *Claim            `json:"claim,omitempty"`
	Dependencies   []Dependency      `json:"dependencies,omitempty"`
	Attempts       []Attempt         `json:"attempts,omitempty"`
	Comments       []Comment         `json:"comments,omitempty"`
	Links          []Link            `json:"links,omitempty"`
	BlockReason    string            `json:"block_reason,omitempty"`
	CreatedMS      int64             `json:"created_ms"`
	UpdatedMS      int64             `json:"updated_ms"`
	CompletedMS    int64             `json:"completed_ms,omitempty"`
	ArchivedMS     int64             `json:"archived_ms,omitempty"`
}

type Claim struct {
	Agent       string `json:"agent"`
	RunID       string `json:"run_id,omitempty"`
	ClaimedMS   int64  `json:"claimed_ms"`
	HeartbeatMS int64  `json:"heartbeat_ms"`
}

type Dependency struct {
	ID        string `json:"id"`
	CreatedMS int64  `json:"created_ms"`
}

type DependencyState struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Status    Status `json:"status"`
	Missing   bool   `json:"missing,omitempty"`
	CreatedMS int64  `json:"created_ms,omitempty"`
}

type Attempt struct {
	ID         string `json:"id"`
	Agent      string `json:"agent,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	Status     string `json:"status"`
	StartedMS  int64  `json:"started_ms"`
	FinishedMS int64  `json:"finished_ms,omitempty"`
	Summary    string `json:"summary,omitempty"`
}

type Comment struct {
	ID        string `json:"id"`
	Author    string `json:"author,omitempty"`
	Body      string `json:"body"`
	CreatedMS int64  `json:"created_ms"`
}

type Link struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Target    string `json:"target"`
	CreatedMS int64  `json:"created_ms"`
}

type RetryPolicy struct {
	MaxAttempts int    `json:"max_attempts,omitempty"`
	EscalateTo  string `json:"escalate_to,omitempty"`
}

type RetryDecision struct {
	Policy       *RetryPolicy `json:"policy,omitempty"`
	FailureCount int          `json:"failure_count"`
	MaxAttempts  int          `json:"max_attempts,omitempty"`
	NextAttempt  int          `json:"next_attempt,omitempty"`
	Retry        bool         `json:"retry"`
	Exhausted    bool         `json:"exhausted"`
	EscalateTo   string       `json:"escalate_to,omitempty"`
	Action       string       `json:"action"`
	Reason       string       `json:"reason,omitempty"`
}

type CreateSpec struct {
	Title          string
	Description    string
	Status         Status
	Priority       int
	Tenant         string
	Assignee       string
	Owner          string
	IdempotencyKey string
	Tags           []string
	Artifacts      []string
	// AcceptanceCriteria are the human-readable conditions the task must satisfy
	// before it may reach done. Declaring any criteria opts the task into the
	// proof gate (see Complete / Prove); leaving it empty keeps the task ungated.
	AcceptanceCriteria []string
	// Seat names the execution preset the task is dispatched under (see
	// kernel/seat). Empty = the assigned agent's defaults. Validation lives at the
	// control plane so this package stays dependency-free.
	Seat        string
	RetryPolicy *RetryPolicy
}

type Filter struct {
	Status          Status
	Tenant          string
	Assignee        string
	IncludeArchived bool
	Limit           int
}

type Store struct {
	path  string
	mu    sync.Mutex
	now   func() time.Time
	tasks []*Task
}

type diskState struct {
	Version int     `json:"version"`
	Tasks   []*Task `json:"tasks"`
}

// OpenStore opens or creates the workboard store under dir.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("workboard: mkdir %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, "workboard.json"), now: time.Now}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("workboard: read %s: %w", s.path, err)
	}
	if len(b) == 0 {
		return s, nil
	}
	var st diskState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("workboard: parse %s: %w", s.path, err)
	}
	for _, t := range st.Tasks {
		if t == nil {
			continue
		}
		if _, ok := statusOrder[t.Status]; !ok {
			return nil, fmt.Errorf("%w in %s: %s", ErrInvalidStatus, s.path, t.Status)
		}
		cp := cloneTask(*t)
		s.tasks = append(s.tasks, &cp)
	}
	return s, nil
}

// Create inserts a task, or returns the existing task when IdempotencyKey is
// set and already present. The bool reports whether a new task was created.
func (s *Store) Create(spec CreateSpec, now time.Time) (Task, bool, error) {
	spec = normalizeCreateSpec(spec)
	if spec.Title == "" {
		return Task{}, false, errors.New("workboard: title required")
	}
	if spec.Status == "" {
		spec.Status = StatusTriage
	}
	if _, ok := statusOrder[spec.Status]; !ok {
		return Task{}, false, fmt.Errorf("%w: %s", ErrInvalidStatus, spec.Status)
	}
	if spec.RetryPolicy != nil && spec.RetryPolicy.MaxAttempts < 1 {
		return Task{}, false, fmt.Errorf("%w: max_attempts must be positive when policy is set", ErrInvalidPolicy)
	}
	ts := now.UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()
	if spec.IdempotencyKey != "" {
		for _, t := range s.tasks {
			if t.IdempotencyKey == spec.IdempotencyKey && t.Tenant == spec.Tenant {
				return cloneTask(*t), false, nil
			}
		}
	}
	if len(s.tasks) >= maxTasks {
		return Task{}, false, fmt.Errorf("workboard: at most %d tasks", maxTasks)
	}
	t := Task{
		ID:             ulid.New(),
		Title:          spec.Title,
		Description:    spec.Description,
		Status:         spec.Status,
		Priority:       spec.Priority,
		Tenant:         spec.Tenant,
		Assignee:       spec.Assignee,
		Owner:          spec.Owner,
		IdempotencyKey: spec.IdempotencyKey,
		Tags:           append([]string(nil), spec.Tags...),
		Artifacts:      append([]string(nil), spec.Artifacts...),
		Criteria:       criteriaFromText(spec.AcceptanceCriteria),
		Seat:           strings.TrimSpace(spec.Seat),
		RetryPolicy:    normalizeRetryPolicy(spec.RetryPolicy),
		CreatedMS:      ts,
		UpdatedMS:      ts,
	}
	s.tasks = append(s.tasks, &t)
	if err := s.saveLocked(); err != nil {
		s.tasks = s.tasks[:len(s.tasks)-1]
		return Task{}, false, err
	}
	return cloneTask(t), true, nil
}

func normalizeCreateSpec(spec CreateSpec) CreateSpec {
	spec.Title = strings.TrimSpace(spec.Title)
	spec.Description = strings.TrimSpace(spec.Description)
	spec.Tenant = strings.TrimSpace(spec.Tenant)
	spec.Assignee = strings.TrimSpace(spec.Assignee)
	spec.Owner = strings.TrimSpace(spec.Owner)
	spec.IdempotencyKey = strings.TrimSpace(spec.IdempotencyKey)
	spec.Tags = cleanStrings(spec.Tags)
	spec.Artifacts = cleanStrings(spec.Artifacts)
	spec.AcceptanceCriteria = cleanStrings(spec.AcceptanceCriteria)
	spec.RetryPolicy = normalizeRetryPolicy(spec.RetryPolicy)
	return spec
}

// criteriaFromText turns the create-time acceptance criteria strings into
// unmet proof.Criterion records. The judge fills Met/Note at prove time.
func criteriaFromText(texts []string) []proof.Criterion {
	if len(texts) == 0 {
		return nil
	}
	out := make([]proof.Criterion, 0, len(texts))
	for _, txt := range texts {
		out = append(out, proof.Criterion{Text: txt})
	}
	return out
}

func normalizeRetryPolicy(policy *RetryPolicy) *RetryPolicy {
	if policy == nil {
		return nil
	}
	cp := *policy
	cp.EscalateTo = strings.TrimSpace(cp.EscalateTo)
	if cp.MaxAttempts <= 0 && cp.EscalateTo == "" {
		return nil
	}
	return &cp
}

func cleanStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func (s *Store) Get(id string) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.find(id); t != nil {
		return cloneTask(*t), true
	}
	return Task{}, false
}

func (s *Store) List(f Filter) []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		if !f.IncludeArchived && t.Status == StatusArchived {
			continue
		}
		if f.Status != "" && t.Status != f.Status {
			continue
		}
		if f.Tenant != "" && t.Tenant != f.Tenant {
			continue
		}
		if f.Assignee != "" && !strings.EqualFold(t.Assignee, f.Assignee) {
			continue
		}
		out = append(out, cloneTask(*t))
	}
	sort.SliceStable(out, func(i, j int) bool {
		ia, ja := out[i].Status == StatusArchived, out[j].Status == StatusArchived
		if ia != ja {
			return !ia
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		if out[i].UpdatedMS != out[j].UpdatedMS {
			return out[i].UpdatedMS > out[j].UpdatedMS
		}
		if statusOrder[out[i].Status] != statusOrder[out[j].Status] {
			return statusOrder[out[i].Status] < statusOrder[out[j].Status]
		}
		return out[i].ID < out[j].ID
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out
}

func (s *Store) Claim(id, agent, runID string, now time.Time) (Task, error) {
	agent = strings.TrimSpace(agent)
	runID = strings.TrimSpace(runID)
	if id == "" || agent == "" {
		return Task{}, errors.New("workboard: claim requires id and agent")
	}
	return s.mutate(id, func(t *Task, ts int64) error {
		if t.Status == StatusArchived || t.Status == StatusDone {
			return fmt.Errorf("workboard: cannot claim %s task", t.Status)
		}
		if t.Claim != nil && !strings.EqualFold(t.Claim.Agent, agent) {
			return ErrClaimConflict
		}
		t.Status = StatusRunning
		if t.Assignee == "" {
			t.Assignee = agent
		}
		t.Claim = &Claim{Agent: agent, RunID: runID, ClaimedMS: ts, HeartbeatMS: ts}
		t.Attempts = append(t.Attempts, Attempt{ID: ulid.New(), Agent: agent, RunID: runID, Status: "running", StartedMS: ts})
		return nil
	}, now)
}

func (s *Store) Heartbeat(id, agent, runID string, now time.Time) (Task, error) {
	agent = strings.TrimSpace(agent)
	runID = strings.TrimSpace(runID)
	return s.mutate(id, func(t *Task, ts int64) error {
		if t.Claim == nil {
			return ErrNotClaimed
		}
		if agent != "" && !strings.EqualFold(t.Claim.Agent, agent) {
			return ErrClaimConflict
		}
		if runID != "" && t.Claim.RunID != "" && t.Claim.RunID != runID {
			return ErrClaimConflict
		}
		t.Claim.HeartbeatMS = ts
		return nil
	}, now)
}

func (s *Store) Comment(id, author, body string, now time.Time) (Task, error) {
	author = strings.TrimSpace(author)
	body = strings.TrimSpace(body)
	if body == "" {
		return Task{}, errors.New("workboard: comment body required")
	}
	return s.mutate(id, func(t *Task, ts int64) error {
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: author, Body: body, CreatedMS: ts})
		return nil
	}, now)
}

func (s *Store) Block(id, actor, reason string, now time.Time) (Task, error) {
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Task{}, errors.New("workboard: block reason required")
	}
	return s.mutate(id, func(t *Task, ts int64) error {
		t.Status = StatusBlocked
		t.BlockReason = reason
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "blocked: " + reason, CreatedMS: ts})
		return nil
	}, now)
}

func (s *Store) Fail(id, actor, reason string, now time.Time) (Task, RetryDecision, error) {
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Task{}, RetryDecision{}, errors.New("workboard: fail reason required")
	}
	var decision RetryDecision
	task, err := s.mutate(id, func(t *Task, ts int64) error {
		markFailedAttempt(t, reason, ts)
		t.Claim = nil
		decision = applyRetryPolicyDecision(t, actor, reason, ts)
		return nil
	}, now)
	if err != nil {
		return Task{}, RetryDecision{}, err
	}
	return task, decision, nil
}

func (s *Store) Unblock(id, actor string, now time.Time) (Task, error) {
	return s.mutate(id, func(t *Task, ts int64) error {
		t.Status = StatusReady
		t.BlockReason = ""
		if actor = strings.TrimSpace(actor); actor != "" {
			t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "unblocked", CreatedMS: ts})
		}
		return nil
	}, now)
}

func (s *Store) SetRetryPolicy(id, actor string, policy *RetryPolicy, now time.Time) (Task, error) {
	actor = strings.TrimSpace(actor)
	policy = normalizeRetryPolicy(policy)
	if policy != nil && policy.MaxAttempts < 1 {
		return Task{}, fmt.Errorf("%w: max_attempts must be positive when policy is set", ErrInvalidPolicy)
	}
	return s.mutate(id, func(t *Task, ts int64) error {
		t.RetryPolicy = policy
		body := "retry policy cleared"
		if policy != nil {
			body = fmt.Sprintf("retry policy set: max_attempts=%d", policy.MaxAttempts)
			if policy.EscalateTo != "" {
				body += " escalate_to=" + policy.EscalateTo
			}
		}
		if actor != "" {
			t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: body, CreatedMS: ts})
		}
		return nil
	}, now)
}

func (s *Store) Complete(id, actor string, now time.Time) (Task, error) {
	return s.mutate(id, func(t *Task, ts int64) error {
		// The proof gate: a task that declared acceptance criteria may only reach
		// done once a recorded Proof satisfies them. Tasks without criteria are
		// ungated and complete as before (default-allow posture).
		if len(t.Criteria) > 0 && (t.Proof == nil || !t.Proof.Satisfied()) {
			return ErrUnproven
		}
		t.Status = StatusDone
		t.CompletedMS = ts
		t.Claim = nil
		for i := len(t.Attempts) - 1; i >= 0; i-- {
			if t.Attempts[i].Status == "running" {
				t.Attempts[i].Status = "done"
				t.Attempts[i].FinishedMS = ts
				break
			}
		}
		if actor = strings.TrimSpace(actor); actor != "" {
			t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "completed", CreatedMS: ts})
		}
		return nil
	}, now)
}

// Prove records a completion proof against the task's acceptance criteria and
// gates the transition to done on it. When the proof is satisfied (verifier
// complete AND every declared criterion met) the task moves to done; otherwise
// it lands in review with the proof attached so the operator sees exactly which
// criteria remain unmet. Prove is the only path a criteria-bearing task takes
// to reach done.
func (s *Store) Prove(id, actor string, p proof.Proof, now time.Time) (Task, error) {
	actor = strings.TrimSpace(actor)
	return s.mutate(id, func(t *Task, ts int64) error {
		if t.Status == StatusArchived {
			return fmt.Errorf("workboard: cannot prove %s task", t.Status)
		}
		if p.ProvedMS == 0 {
			p.ProvedMS = ts
		}
		// Merge the judge's per-criterion outcomes back onto the task's declared
		// criteria (matched by text so create-time order and wording are kept).
		if len(t.Criteria) > 0 {
			p.Criteria = reconcileCriteria(t.Criteria, p.Criteria)
			t.Criteria = append([]proof.Criterion(nil), p.Criteria...)
		}
		cp := p.Clone()
		t.Proof = &cp
		t.Claim = nil
		if p.Satisfied() {
			t.Status = StatusDone
			t.CompletedMS = ts
			finishRunningAttempt(t, "done", ts, "")
			if actor != "" {
				t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "proven complete", CreatedMS: ts})
			}
			return nil
		}
		t.Status = StatusReview
		// An unproven task must not carry a stale completion time (a re-prove of a
		// previously-done task can land here).
		t.CompletedMS = 0
		finishRunningAttempt(t, "review", ts, p.Verdict.Gap)
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "unproven: " + provenGapSummary(p), CreatedMS: ts})
		return nil
	}, now)
}

// reconcileCriteria returns the declared criteria in their original order with
// Met/Note taken from the judged list (matched by trimmed, case-insensitive
// text). A declared criterion the judge did not address stays unmet.
func reconcileCriteria(declared, judged []proof.Criterion) []proof.Criterion {
	out := make([]proof.Criterion, len(declared))
	copy(out, declared)
	for i := range out {
		out[i].Met = false
		out[i].Note = ""
		for _, j := range judged {
			if strings.EqualFold(strings.TrimSpace(j.Text), strings.TrimSpace(out[i].Text)) {
				out[i].Met = j.Met
				out[i].Note = j.Note
				break
			}
		}
	}
	return out
}

// provenGapSummary produces a one-line reason a proof failed the gate.
func provenGapSummary(p proof.Proof) string {
	if g := strings.TrimSpace(p.Verdict.Gap); g != "" {
		return g
	}
	if n := p.UnmetCount(); n > 0 {
		return fmt.Sprintf("%d of %d criteria unmet", n, len(p.Criteria))
	}
	return "verifier did not confirm completion"
}

// finishRunningAttempt closes the most recent running attempt with the given
// terminal status (and optional summary), mirroring the inline loops used by
// Complete/Review.
func finishRunningAttempt(t *Task, status string, ts int64, summary string) {
	for i := len(t.Attempts) - 1; i >= 0; i-- {
		if t.Attempts[i].Status == "running" {
			t.Attempts[i].Status = status
			t.Attempts[i].FinishedMS = ts
			if summary != "" {
				t.Attempts[i].Summary = summary
			}
			return
		}
	}
}

func (s *Store) Review(id, actor, summary string, now time.Time) (Task, error) {
	actor = strings.TrimSpace(actor)
	summary = strings.TrimSpace(summary)
	return s.mutate(id, func(t *Task, ts int64) error {
		t.Status = StatusReview
		t.Claim = nil
		for i := len(t.Attempts) - 1; i >= 0; i-- {
			if t.Attempts[i].Status == "running" {
				t.Attempts[i].Status = "review"
				t.Attempts[i].FinishedMS = ts
				t.Attempts[i].Summary = summary
				break
			}
		}
		body := "ready for review"
		if summary != "" {
			body += ": " + summary
		}
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: body, CreatedMS: ts})
		return nil
	}, now)
}

// SetSeat changes the execution seat a task will be dispatched under. An empty
// seat clears it (back to the agent's defaults). Seat-id validity is enforced at
// the control plane; the store just records the string.
func (s *Store) SetSeat(id, seat string, now time.Time) (Task, error) {
	seat = strings.TrimSpace(seat)
	return s.mutate(id, func(t *Task, _ int64) error {
		t.Seat = seat
		return nil
	}, now)
}

func (s *Store) Archive(id, actor string, now time.Time) (Task, error) {
	return s.mutate(id, func(t *Task, ts int64) error {
		t.Status = StatusArchived
		t.ArchivedMS = ts
		t.Claim = nil
		if actor = strings.TrimSpace(actor); actor != "" {
			t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "archived", CreatedMS: ts})
		}
		return nil
	}, now)
}

func (s *Store) Link(id, typ, target string, now time.Time) (Task, error) {
	typ = strings.TrimSpace(typ)
	target = strings.TrimSpace(target)
	if typ == "" || target == "" {
		return Task{}, errors.New("workboard: link requires type and target")
	}
	return s.mutate(id, func(t *Task, ts int64) error {
		t.Links = append(t.Links, Link{ID: ulid.New(), Type: typ, Target: target, CreatedMS: ts})
		return nil
	}, now)
}

func (s *Store) AddDependency(id, dependsOn string, now time.Time) (Task, error) {
	id = strings.TrimSpace(id)
	dependsOn = strings.TrimSpace(dependsOn)
	if id == "" || dependsOn == "" {
		return Task{}, errors.New("workboard: dependency requires id and depends_on")
	}
	if id == dependsOn {
		return Task{}, errors.New("workboard: task cannot depend on itself")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil {
		return Task{}, ErrNotFound
	}
	dep := s.find(dependsOn)
	if dep == nil {
		return Task{}, fmt.Errorf("workboard: dependency not found: %s", dependsOn)
	}
	if s.dependsOnLocked(dep.ID, t.ID, map[string]bool{}) {
		return Task{}, errors.New("workboard: dependency would create a cycle")
	}
	prev := cloneTask(*t)
	for _, d := range t.Dependencies {
		if d.ID == dep.ID {
			return cloneTask(*t), nil
		}
	}
	ts := now.UnixMilli()
	t.Dependencies = append(t.Dependencies, Dependency{ID: dep.ID, CreatedMS: ts})
	t.UpdatedMS = ts
	if err := s.saveLocked(); err != nil {
		*t = prev
		return Task{}, err
	}
	return cloneTask(*t), nil
}

func (s *Store) BlockingDependencies(id string) ([]DependencyState, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil {
		return nil, ErrNotFound
	}
	var out []DependencyState
	for _, d := range t.Dependencies {
		dep := s.find(d.ID)
		if dep == nil {
			out = append(out, DependencyState{ID: d.ID, Status: "missing", Missing: true, CreatedMS: d.CreatedMS})
			continue
		}
		if dependencySatisfied(*dep) {
			continue
		}
		out = append(out, DependencyState{ID: dep.ID, Title: dep.Title, Status: dep.Status, CreatedMS: d.CreatedMS})
	}
	return out, nil
}

func (s *Store) ReclaimStale(id, actor string, staleAfter time.Duration, now time.Time) (Task, error) {
	actor = strings.TrimSpace(actor)
	if staleAfter <= 0 {
		return Task{}, errors.New("workboard: stale_after must be positive")
	}
	return s.mutate(id, func(t *Task, ts int64) error {
		return reclaimStaleTask(t, actor, staleAfter, ts)
	}, now)
}

func (s *Store) SweepStaleClaims(actor string, staleAfter time.Duration, limit int, now time.Time) ([]Task, error) {
	actor = strings.TrimSpace(actor)
	if staleAfter <= 0 {
		return nil, errors.New("workboard: stale_after must be positive")
	}
	if limit <= 0 {
		limit = maxTasks
	}
	ts := now.UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := make([]Task, len(s.tasks))
	for i, t := range s.tasks {
		prev[i] = cloneTask(*t)
	}
	out := make([]Task, 0)
	for _, t := range s.tasks {
		if len(out) >= limit {
			break
		}
		if t.Claim == nil || ts-t.Claim.HeartbeatMS < staleAfter.Milliseconds() {
			continue
		}
		if err := reclaimStaleTask(t, actor, staleAfter, ts); err != nil {
			continue
		}
		t.UpdatedMS = ts
		out = append(out, cloneTask(*t))
	}
	if len(out) == 0 {
		return nil, nil
	}
	if err := s.saveLocked(); err != nil {
		for i := range s.tasks {
			*s.tasks[i] = prev[i]
		}
		return nil, err
	}
	return out, nil
}

func (s *Store) mutate(id string, fn func(*Task, int64) error, now time.Time) (Task, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Task{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil {
		return Task{}, ErrNotFound
	}
	prev := cloneTask(*t)
	ts := now.UnixMilli()
	if err := fn(t, ts); err != nil {
		*t = prev
		return Task{}, err
	}
	t.UpdatedMS = ts
	if err := s.saveLocked(); err != nil {
		*t = prev
		return Task{}, err
	}
	return cloneTask(*t), nil
}

func (s *Store) find(id string) *Task {
	for _, t := range s.tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(diskState{Version: storeVersion, Tasks: s.tasks}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		if stdruntime.GOOS == "windows" {
			if removeErr := os.Remove(s.path); removeErr == nil || os.IsNotExist(removeErr) {
				if retryErr := os.Rename(tmp, s.path); retryErr == nil {
					return nil
				} else {
					err = retryErr
				}
			}
		}
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *Store) dependsOnLocked(startID, targetID string, seen map[string]bool) bool {
	if startID == targetID {
		return true
	}
	if seen[startID] {
		return false
	}
	seen[startID] = true
	t := s.find(startID)
	if t == nil {
		return false
	}
	for _, d := range t.Dependencies {
		if s.dependsOnLocked(d.ID, targetID, seen) {
			return true
		}
	}
	return false
}

func dependencySatisfied(t Task) bool {
	return t.Status == StatusDone || t.CompletedMS > 0
}

func FailedAttemptCount(t Task) int {
	n := 0
	for _, a := range t.Attempts {
		if attemptCountsAsFailure(a.Status) {
			n++
		}
	}
	return n
}

func RetryDecisionFor(t Task, reason string) RetryDecision {
	return retryDecision(t.RetryPolicy, FailedAttemptCount(t), strings.TrimSpace(reason))
}

func attemptCountsAsFailure(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "stale":
		return true
	default:
		return false
	}
}

func markFailedAttempt(t *Task, reason string, ts int64) {
	for i := len(t.Attempts) - 1; i >= 0; i-- {
		if t.Attempts[i].Status == "running" {
			t.Attempts[i].Status = "failed"
			t.Attempts[i].FinishedMS = ts
			t.Attempts[i].Summary = reason
			return
		}
	}
	agent, runID := "", ""
	if t.Claim != nil {
		agent = t.Claim.Agent
		runID = t.Claim.RunID
	}
	t.Attempts = append(t.Attempts, Attempt{
		ID:         ulid.New(),
		Agent:      agent,
		RunID:      runID,
		Status:     "failed",
		StartedMS:  ts,
		FinishedMS: ts,
		Summary:    reason,
	})
}

func applyRetryPolicyDecision(t *Task, actor, reason string, ts int64) RetryDecision {
	decision := RetryDecisionFor(*t, reason)
	switch {
	case decision.Retry:
		t.Status = StatusReady
		t.BlockReason = ""
		t.Comments = append(t.Comments, Comment{
			ID:        ulid.New(),
			Author:    actor,
			Body:      fmt.Sprintf("retry scheduled: attempt %d/%d after failure: %s", decision.NextAttempt, decision.MaxAttempts, reason),
			CreatedMS: ts,
		})
	case decision.Exhausted:
		t.Status = StatusBlocked
		t.BlockReason = fmt.Sprintf("retry exhausted after %d/%d attempts: %s", decision.FailureCount, decision.MaxAttempts, reason)
		if decision.EscalateTo != "" {
			t.BlockReason = fmt.Sprintf("retry exhausted after %d/%d attempts; escalate to %s: %s", decision.FailureCount, decision.MaxAttempts, decision.EscalateTo, reason)
		}
		body := "escalated: " + t.BlockReason
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: body, CreatedMS: ts})
	default:
		t.Status = StatusBlocked
		t.BlockReason = reason
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "failed: " + reason, CreatedMS: ts})
	}
	return decision
}

func retryDecision(policy *RetryPolicy, failureCount int, reason string) RetryDecision {
	decision := RetryDecision{FailureCount: failureCount, Reason: reason, Action: "block"}
	policy = normalizeRetryPolicy(policy)
	if policy == nil || policy.MaxAttempts <= 0 {
		return decision
	}
	cp := *policy
	decision.Policy = &cp
	decision.MaxAttempts = cp.MaxAttempts
	decision.EscalateTo = cp.EscalateTo
	if failureCount < cp.MaxAttempts {
		decision.Retry = true
		decision.NextAttempt = failureCount + 1
		decision.Action = "retry"
		return decision
	}
	decision.Exhausted = true
	decision.Action = "escalate"
	return decision
}

func reclaimStaleTask(t *Task, actor string, staleAfter time.Duration, ts int64) error {
	if t.Claim == nil {
		return ErrNotClaimed
	}
	if ts-t.Claim.HeartbeatMS < staleAfter.Milliseconds() {
		return ErrClaimFresh
	}
	old := *t.Claim
	t.Status = StatusReady
	t.Claim = nil
	for i := len(t.Attempts) - 1; i >= 0; i-- {
		if t.Attempts[i].Status == "running" && strings.EqualFold(t.Attempts[i].Agent, old.Agent) && (old.RunID == "" || t.Attempts[i].RunID == old.RunID) {
			t.Attempts[i].Status = "stale"
			t.Attempts[i].FinishedMS = ts
			t.Attempts[i].Summary = "reclaimed after stale heartbeat"
			break
		}
	}
	body := "reclaimed stale claim"
	if old.Agent != "" {
		body += " from " + old.Agent
	}
	t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: body, CreatedMS: ts})
	if decision := RetryDecisionFor(*t, "stale heartbeat"); decision.Exhausted {
		t.Status = StatusBlocked
		t.BlockReason = fmt.Sprintf("retry exhausted after %d/%d attempts: stale heartbeat", decision.FailureCount, decision.MaxAttempts)
		if decision.EscalateTo != "" {
			t.BlockReason = fmt.Sprintf("retry exhausted after %d/%d attempts; escalate to %s: stale heartbeat", decision.FailureCount, decision.MaxAttempts, decision.EscalateTo)
		}
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: "escalated: " + t.BlockReason, CreatedMS: ts})
	} else if decision.Retry {
		t.Comments = append(t.Comments, Comment{ID: ulid.New(), Author: actor, Body: fmt.Sprintf("retry scheduled: attempt %d/%d after stale heartbeat", decision.NextAttempt, decision.MaxAttempts), CreatedMS: ts})
	}
	return nil
}

func cloneTask(t Task) Task {
	t.Tags = append([]string(nil), t.Tags...)
	t.Artifacts = append([]string(nil), t.Artifacts...)
	t.Criteria = append([]proof.Criterion(nil), t.Criteria...)
	if t.Proof != nil {
		cp := t.Proof.Clone()
		t.Proof = &cp
	}
	if t.RetryPolicy != nil {
		cp := *t.RetryPolicy
		t.RetryPolicy = &cp
	}
	t.Dependencies = append([]Dependency(nil), t.Dependencies...)
	t.Attempts = append([]Attempt(nil), t.Attempts...)
	t.Comments = append([]Comment(nil), t.Comments...)
	t.Links = append([]Link(nil), t.Links...)
	if t.Claim != nil {
		cp := *t.Claim
		t.Claim = &cp
	}
	return t
}
