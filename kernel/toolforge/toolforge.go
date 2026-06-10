// SPDX-License-Identifier: MIT

// Package toolforge is the script-tool forge (M794): agent-authored code
// promoted into durable, callable tools — the close of the write→use→improve
// cycle. An agent (or operator) DRAFTS a named script, TESTS it in the
// code-exec sandbox, and once a test of the current code has passed the
// OPERATOR promotes it; from then on every run is offered the script as a
// real tool named `forge_<name>`, executed through the same warden-isolated,
// secret-scrubbed sandbox as `code_exec` and gated by the same `code.exec`
// Edict capability. Quarantine is the instant kill switch, and ANY edit to
// the code demotes the tool back to draft with its test record cleared —
// only tested code is ever live.
//
// Storage mirrors kernel/roster: a single JSON file rewritten atomically on
// change, safe for concurrent use; every lifecycle mutation is journaled by
// the kernel (scripttool.*) so `agt why` can explain how a tool came to be.
package toolforge

import (
	"context"
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

// ErrNotFound is returned for an unknown script-tool id/name.
var ErrNotFound = errors.New("toolforge: script tool not found")

// ErrUntested is returned by Promote when the CURRENT code has no passing
// test on record — the forge's core invariant: only tested code goes live.
var ErrUntested = errors.New("toolforge: the current code has no passing test — run a test first")

// Status is a script tool's lifecycle state.
type Status string

const (
	// StatusDraft: authored but not live — never offered to runs.
	StatusDraft Status = "draft"
	// StatusActive: promoted — offered to every run as forge_<name>.
	StatusActive Status = "active"
	// StatusQuarantined: pulled from production (kill switch); re-promotable.
	StatusQuarantined Status = "quarantined"
)

// ScriptTool is one named, durable script the agent can call as a tool.
type ScriptTool struct {
	ID string `json:"id"`
	// Name is the tool's immutable handle; runs see it as `forge_<name>`.
	Name string `json:"name"`
	// Description is what the MODEL reads to decide when to call it.
	Description string `json:"description"`
	// Language is the sandbox runtime id ("python", "node", "deno").
	// Availability is judged at test/run time by the sandbox itself.
	Language string `json:"language"`
	// Code is the script body. Contract: the invoking call's JSON input is
	// written to ./stdin.txt in the work dir; stdout becomes the tool result.
	Code string `json:"code"`
	// InputSchema is an optional JSON-Schema object describing the call
	// input; empty means a permissive default schema.
	InputSchema string `json:"input_schema,omitempty"`

	Status Status `json:"status"`
	// TestedOK records whether the CURRENT code has a passing sandbox test;
	// cleared by any code/language edit. Promote requires it.
	TestedOK bool  `json:"tested_ok"`
	TestedMS int64 `json:"tested_ms,omitempty"`

	CreatedMS int64 `json:"created_ms"`
	UpdatedMS int64 `json:"updated_ms"`
}

// Runner executes a script in the code-exec sandbox: the call's raw JSON
// input rides in as inputJSON (surfaced to the script as ./stdin.txt) and
// combined stdout+stderr comes back. isError mirrors the sandbox's own
// verdict (non-zero exit, timeout, unavailable language...). Implemented by
// the code_exec tool; the daemon is the single wiring point.
type Runner interface {
	RunScript(ctx context.Context, language, code, inputJSON string) (output string, isError bool, err error)
}

// nameRe: a tool-name token — lowercase start, then letters/digits/underscore.
// `forge_` + 40 chars stays well inside provider tool-name limits.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

var langRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,15}$`)

const (
	maxCodeBytes   = 128 * 1024 // a tool, not a codebase
	maxDescBytes   = 2 * 1024
	maxSchemaBytes = 16 * 1024
)

// Validate checks a script tool's user-supplied fields (identity/lifecycle
// fields are kernel-assigned and not judged here).
func Validate(st ScriptTool) error {
	if !nameRe.MatchString(st.Name) {
		return fmt.Errorf("toolforge: name must match %s", nameRe)
	}
	if strings.TrimSpace(st.Description) == "" {
		return errors.New("toolforge: description is required (the model reads it to decide when to call the tool)")
	}
	if len(st.Description) > maxDescBytes {
		return fmt.Errorf("toolforge: description exceeds %d bytes", maxDescBytes)
	}
	if !langRe.MatchString(st.Language) {
		return errors.New("toolforge: language is required (a sandbox runtime id, e.g. python/node/deno)")
	}
	if strings.TrimSpace(st.Code) == "" {
		return errors.New("toolforge: code is required")
	}
	if len(st.Code) > maxCodeBytes {
		return fmt.Errorf("toolforge: code exceeds %d bytes", maxCodeBytes)
	}
	if s := strings.TrimSpace(st.InputSchema); s != "" {
		if len(s) > maxSchemaBytes {
			return fmt.Errorf("toolforge: input_schema exceeds %d bytes", maxSchemaBytes)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(s), &obj); err != nil {
			return fmt.Errorf("toolforge: input_schema must be a JSON object: %w", err)
		}
	}
	return nil
}

// Store is the persistent script-tool registry, a single JSON file rewritten
// atomically on change. Safe for concurrent use. Mirrors kernel/roster.Store.
type Store struct {
	path  string
	mu    sync.Mutex
	now   func() time.Time
	tools []*ScriptTool
}

// Open opens (or creates) the script-tool store under dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("toolforge: mkdir %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, "scripttools.json"), now: time.Now}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("toolforge: read %s: %w", s.path, err)
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.tools); err != nil {
			return nil, fmt.Errorf("toolforge: parse %s: %w", s.path, err)
		}
	}
	return s, nil
}

// Add validates and persists a new DRAFT script tool, assigning an id +
// timestamps. Caller-supplied ID/Status/Tested*/timestamps are ignored
// (kernel-assigned). The name must be unique across the forge.
func (s *Store) Add(st ScriptTool) (ScriptTool, error) {
	st.Name = strings.TrimSpace(st.Name)
	st.Language = strings.TrimSpace(st.Language)
	if err := Validate(st); err != nil {
		return ScriptTool{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ex := range s.tools {
		if ex.Name == st.Name {
			return ScriptTool{}, fmt.Errorf("toolforge: name %q already exists", st.Name)
		}
	}
	now := s.now().UnixMilli()
	st.ID = ulid.New()
	st.Status = StatusDraft
	st.TestedOK = false
	st.TestedMS = 0
	st.CreatedMS = now
	st.UpdatedMS = now
	cp := st
	s.tools = append(s.tools, &cp)
	if err := s.save(); err != nil {
		s.tools = s.tools[:len(s.tools)-1]
		return ScriptTool{}, err
	}
	return cp, nil
}

// Update applies edits to a script tool's mutable fields via mutate,
// re-validates, and persists. Identity and lifecycle fields — ID, Name,
// CreatedMS, Status, TestedOK/TestedMS — are preserved regardless of what
// mutate does, EXCEPT that changing Code or Language demotes the tool to
// draft and clears its test record: only tested code is ever live. Rolled
// back in memory on validation/save failure.
func (s *Store) Update(ref string, mutate func(*ScriptTool)) (ScriptTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.find(ref)
	if st == nil {
		return ScriptTool{}, ErrNotFound
	}
	snapshot := *st
	mutate(st)
	// Protect identity + lifecycle fields from the mutator: the name is the
	// tool's ADDRESS, and status/test records move only through their own
	// governed transitions.
	st.ID, st.Name, st.CreatedMS = snapshot.ID, snapshot.Name, snapshot.CreatedMS
	st.Status, st.TestedOK, st.TestedMS = snapshot.Status, snapshot.TestedOK, snapshot.TestedMS
	st.Language = strings.TrimSpace(st.Language)
	if st.Code != snapshot.Code || st.Language != snapshot.Language {
		st.Status = StatusDraft
		st.TestedOK = false
		st.TestedMS = 0
	}
	st.UpdatedMS = s.now().UnixMilli()
	if err := Validate(*st); err != nil {
		*st = snapshot
		return ScriptTool{}, err
	}
	if err := s.save(); err != nil {
		*st = snapshot
		return ScriptTool{}, err
	}
	return *st, nil
}

// RecordTest stamps the outcome of a sandbox test of the CURRENT code.
func (s *Store) RecordTest(ref string, ok bool) (ScriptTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.find(ref)
	if st == nil {
		return ScriptTool{}, ErrNotFound
	}
	prevOK, prevMS := st.TestedOK, st.TestedMS
	st.TestedOK = ok
	st.TestedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		st.TestedOK, st.TestedMS = prevOK, prevMS
		return ScriptTool{}, err
	}
	return *st, nil
}

// Promote moves a draft or quarantined tool to ACTIVE — from then on every
// run is offered it as forge_<name>. Refused with ErrUntested unless the
// current code has a passing test on record.
func (s *Store) Promote(ref string) (ScriptTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.find(ref)
	if st == nil {
		return ScriptTool{}, ErrNotFound
	}
	if st.Status == StatusActive {
		return ScriptTool{}, fmt.Errorf("toolforge: %s is already active", st.Name)
	}
	if !st.TestedOK {
		return ScriptTool{}, ErrUntested
	}
	prevStatus, prevUpdated := st.Status, st.UpdatedMS
	st.Status = StatusActive
	st.UpdatedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		st.Status, st.UpdatedMS = prevStatus, prevUpdated
		return ScriptTool{}, err
	}
	return *st, nil
}

// Quarantine pulls an ACTIVE tool from production — the kill switch. The
// test record survives, so an un-edited tool can be re-promoted directly.
func (s *Store) Quarantine(ref string) (ScriptTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.find(ref)
	if st == nil {
		return ScriptTool{}, ErrNotFound
	}
	if st.Status != StatusActive {
		return ScriptTool{}, fmt.Errorf("toolforge: %s is %s, not active", st.Name, st.Status)
	}
	prevStatus, prevUpdated := st.Status, st.UpdatedMS
	st.Status = StatusQuarantined
	st.UpdatedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		st.Status, st.UpdatedMS = prevStatus, prevUpdated
		return ScriptTool{}, err
	}
	return *st, nil
}

// Remove deletes a script tool by id or name. Returns whether it existed.
func (s *Store) Remove(ref string) (ScriptTool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, st := range s.tools {
		if st.ID == ref || st.Name == ref {
			removed := s.tools
			gone := *st
			s.tools = append(append([]*ScriptTool{}, s.tools[:i]...), s.tools[i+1:]...)
			if err := s.save(); err != nil {
				s.tools = removed // restore: disk write failed, keep the tool
				return ScriptTool{}, false, err
			}
			return gone, true, nil
		}
	}
	return ScriptTool{}, false, nil
}

// Get returns one script tool by id or name.
func (s *Store) Get(ref string) (ScriptTool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.find(ref); st != nil {
		return *st, true
	}
	return ScriptTool{}, false
}

// find returns the live pointer for an id or name. Caller holds s.mu.
func (s *Store) find(ref string) *ScriptTool {
	for _, st := range s.tools {
		if st.ID == ref || st.Name == ref {
			return st
		}
	}
	return nil
}

// List returns all script tools, sorted by creation time then id.
func (s *Store) List() []ScriptTool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ScriptTool, 0, len(s.tools))
	for _, st := range s.tools {
		out = append(out, *st)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedMS != out[j].CreatedMS {
			return out[i].CreatedMS < out[j].CreatedMS
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Active returns only the tools currently offered to runs, sorted like List.
func (s *Store) Active() []ScriptTool {
	all := s.List()
	out := all[:0]
	for _, st := range all {
		if st.Status == StatusActive {
			out = append(out, st)
		}
	}
	return out
}

// Count returns the number of script tools.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tools)
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.tools, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
