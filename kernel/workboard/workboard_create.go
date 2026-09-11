// SPDX-License-Identifier: MIT

package workboard

// OpenStore + Create + create-spec normalization helpers (normalizeCreateSpec
// + criteriaFromText + normalizeRetryPolicy + cleanStrings). Carved out
// of workboard.go during the Day 36 god file split #1.

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/jsonstore"
	"github.com/agezt/agezt/kernel/proof"
	"github.com/agezt/agezt/kernel/ulid"
)

func OpenStore(dir string) (*Store, error) {
	s := &Store{now: time.Now}
	var st diskState
	path, err := jsonstore.LoadFrom(dir, "workboard.json", &st)
	if err != nil {
		return nil, fmt.Errorf("workboard: %w", err)
	}
	s.path = path
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
