// SPDX-License-Identifier: MIT

// Package memory implements the memory store (ROADMAP §2.3): a journaled,
// content-addressed knowledge store that the agent loop reads as injected
// context and that the operator, the agent, and an auto-distiller can write
// to. Retrieval is hybrid (M803): exact keyword overlap blended with local
// hashed-n-gram embeddings (vector.go) for typo/morphology recall —
// DECISIONS C5's "local embeddings by default"; provider embeddings remain
// the documented opt-in.
//
// Two layers, mirroring how kernel/state and kernel/runtime split:
//
//   - Store (this file) is a pure, file-backed record store — no bus, no
//     journaling — so it is trivially testable and a CobaltDB-class engine
//     (DECISIONS D2) can replace it behind the interface. It also owns the
//     content-addressing and the keyword retrieval ranking, both pure
//     functions.
//   - Manager (manager.go) wraps a Store with the kernel bus so every
//     mutation is a durable-before-publish event carrying the run's
//     correlation_id — which is what makes `agt why` able to explain every
//     belief (SPEC-05 §2).
//
// Records are content-addressed (BLAKE3 of type\0subject\0content) so
// identical knowledge dedupes; updates are soft (SupersededBy) and forgets
// are soft (Tombstoned) — history is never destructively edited.
//
// Concurrency: a single Store instance is safe for concurrent use.
package memory

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"lukechampine.com/blake3"
)

// Type is the canonical memory-record discriminator. The set mirrors
// SPEC-05 §2 MemoryType, minus SKILL_REF (skills land with Forge).
type Type string

const (
	TypeFact        Type = "FACT"        // a durable fact worth recalling
	TypeSummary     Type = "SUMMARY"     // a distilled summary of work done
	TypeRelation    Type = "RELATION"    // a relation between entities
	TypePreference  Type = "PREFERENCE"  // a stated/learned user preference
	TypeObservation Type = "OBSERVATION" // something noticed, lower confidence
)

// DefaultType is used when a writer leaves the type unset.
const DefaultType = TypeFact

// Evidence records how a memory entered the store. It is deliberately
// orthogonal to Type: "OBSERVATION" as a Type means the content is a note;
// EvidenceObserved means the note was grounded in a direct observation.
type Evidence string

const (
	EvidenceUnknown    Evidence = ""
	EvidenceObserved   Evidence = "observed"
	EvidenceInferred   Evidence = "inferred"
	EvidenceCurated    Evidence = "curated"
	EvidenceConstraint Evidence = "constraint"
)

// validTypes is the membership set for ValidType.
var validTypes = map[Type]struct{}{
	TypeFact: {}, TypeSummary: {}, TypeRelation: {},
	TypePreference: {}, TypeObservation: {},
}

// ValidType reports whether t is one of the known memory types.
func ValidType(t Type) bool {
	_, ok := validTypes[t]
	return ok
}

// Record is one unit of memory. Field order is not load-bearing (records are
// not part of the event hash chain), but the JSON tags are stable so the
// on-disk file and the CLI/`--json` shape stay compatible across releases.
type Record struct {
	// ID is content-addressed: hex(BLAKE3(type \0 subject \0 content)).
	ID string `json:"id"`
	// Type classifies the knowledge (FACT, SUMMARY, ...).
	Type Type `json:"type"`
	// Subject is the entity/topic this is about (used for retrieval).
	Subject string `json:"subject"`
	// Content is the text the model sees when this record is injected.
	Content string `json:"content"`
	// Tags are free-form labels (e.g. source=distill, project=lictor).
	Tags map[string]string `json:"tags,omitempty"`
	// SourceEvent is the journal event id that produced this record —
	// provenance for `agt why`.
	SourceEvent string `json:"source_event,omitempty"`
	// AddedBy / UpdatedBy record WHO wrote this (M851): the acting agent's slug,
	// or "operator" for a direct console/CLI write, or "distill" for an
	// auto-distilled summary. AddedBy is first-writer-wins (preserved on
	// reinforce, like SourceEvent); UpdatedBy is the most recent writer. Empty on
	// legacy records written before provenance.
	AddedBy   string `json:"added_by,omitempty"`
	UpdatedBy string `json:"updated_by,omitempty"`
	// Confidence is 0..1; ranking weights it and it can strengthen on
	// re-observation. Defaults to 1.0 for explicit writes.
	Confidence float64 `json:"confidence"`
	// Evidence distinguishes observed, inferred, curated, and constraint-like
	// memories. Retrieval treats this as metadata, not as truth authority.
	Evidence Evidence `json:"evidence,omitempty"`
	// CreatedMS / LastSeenMS drive recency in ranking and decay.
	CreatedMS  int64 `json:"created_ms"`
	LastSeenMS int64 `json:"last_seen_ms"`
	// HalfLifeMS is the mechanical expiration budget. Once LastSeenMS+HalfLifeMS
	// is in the past, the record is retained but barred from active use until it
	// is reconstructed/reinforced.
	HalfLifeMS int64 `json:"half_life_ms,omitempty"`
	// SuspendedMS marks a record that failed an epistemic hygiene check. It is
	// retained and gettable, but excluded from recall/search.
	SuspendedMS     int64  `json:"suspended_ms,omitempty"`
	SuspendedReason string `json:"suspended_reason,omitempty"`
	// SupersededBy points at a newer record when this one was replaced
	// (soft update — the old record is retained, not edited away).
	SupersededBy string `json:"superseded_by,omitempty"`
	// Tombstoned marks a soft-forgotten record: excluded from recall but
	// retained on disk and in the journal (reversibility).
	Tombstoned bool `json:"tombstoned,omitempty"`
}

// Active reports whether the record should participate in retrieval:
// neither forgotten nor superseded.
func (r Record) Active() bool { return !r.Tombstoned && r.SupersededBy == "" }

// Suspended reports whether the record is explicitly barred from active use.
func (r Record) Suspended() bool { return r.SuspendedMS > 0 }

// Expired reports whether the record's reconstruction budget has elapsed.
// A zero HalfLifeMS means legacy/no-expiry records remain usable.
func (r Record) Expired(nowMS int64) bool {
	return r.HalfLifeMS > 0 && nowMS > r.LastSeenMS+r.HalfLifeMS
}

// Usable reports whether the record may participate in retrieval at nowMS.
func (r Record) Usable(nowMS int64) bool {
	return r.Active() && !r.Suspended() && !r.Expired(nowMS)
}

// ContentID computes the content-addressed id for a (type, subject, content)
// triple. NUL separators avoid ("ab","c") colliding with ("a","bc").
func ContentID(t Type, subject, content string) string {
	h := blake3.New(32, nil)
	h.Write([]byte(string(t)))
	h.Write([]byte{0})
	h.Write([]byte(subject))
	h.Write([]byte{0})
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// ScopedID extends ContentID with the record's scope (M915): the same fact
// written privately by two different agents must yield two records, not one
// record whose scope tag flips to the latest writer (silently hiding the note
// from the first agent). An empty scope hashes identically to ContentID, so
// every pre-existing shared record keeps its id.
func ScopedID(t Type, subject, content, scope string) string {
	if scope == "" {
		return ContentID(t, subject, content)
	}
	h := blake3.New(32, nil)
	h.Write([]byte(string(t)))
	h.Write([]byte{0})
	h.Write([]byte(subject))
	h.Write([]byte{0})
	h.Write([]byte(content))
	h.Write([]byte{0})
	h.Write([]byte(scope))
	return hex.EncodeToString(h.Sum(nil))
}

// Store is the pure record store. Implementations persist records by id and
// must be safe for concurrent use.
type Store interface {
	// Put upserts r by its ID (overwrites any existing record with that id).
	Put(r Record) error
	// Get returns the record at id; the bool is false if absent.
	Get(id string) (Record, bool, error)
	// Delete hard-removes the record at id (returns false if absent). Unlike
	// Forget (which tombstones), this reclaims the row — used only by the prune
	// pass on already-soft-deleted records (M857).
	Delete(id string) (bool, error)
	// All returns every record (including tombstoned/superseded ones),
	// sorted deterministically by CreatedMS then ID.
	All() ([]Record, error)
	// Count returns the number of records currently stored (all states).
	Count() int
	// Close releases any held resources.
	Close() error
}

// ErrEmptyContent is returned by callers that try to store a record with no
// content; the Manager validates before reaching the store, but the store
// guards too.
var ErrEmptyContent = errors.New("memory: empty content")

// FileStore is the file-backed Store. All records live in a single
// <dir>/memory.json object keyed by id, snapshotted atomically on every
// mutation (write-temp + rename). Simple and crash-safe; not optimized for
// high write volume — adequate for memory-lite.
type FileStore struct {
	path string

	mu   sync.RWMutex
	data map[string]Record
}

// Open opens (or creates) a FileStore under dir, loading <dir>/memory.json
// if present. The directory is created if absent.
func Open(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("memory: mkdir %s: %w", dir, err)
	}
	s := &FileStore{
		path: filepath.Join(dir, "memory.json"),
		data: make(map[string]Record),
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("memory: read %s: %w", s.path, err)
	}
	if len(raw) == 0 {
		return s, nil
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(raw)) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("memory: parse %s: %w", s.path, err)
	}
	if s.data == nil {
		s.data = make(map[string]Record)
	}
	return s, nil
}

// Put implements Store.
func (s *FileStore) Put(r Record) error {
	if r.ID == "" {
		return errors.New("memory: record id required")
	}
	if strings.TrimSpace(r.Content) == "" {
		return ErrEmptyContent
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[r.ID] = r
	return s.snapshotLocked()
}

// Get implements Store.
func (s *FileStore) Get(id string) (Record, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.data[id]
	return r, ok, nil
}

// Delete implements Store — a hard removal (M857). Returns false if id is absent.
func (s *FileStore) Delete(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return false, nil
	}
	delete(s.data, id)
	return true, s.snapshotLocked()
}

// All implements Store. The returned slice is sorted by CreatedMS then ID so
// two consecutive calls produce identical output (load-bearing for snapshot
// tests and deterministic CLI rendering).
func (s *FileStore) All() ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.data))
	for _, r := range s.data {
		out = append(out, r)
	}
	sortRecords(out)
	return out, nil
}

// Count implements Store.
func (s *FileStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}

// Close implements Store. Mutations persist synchronously, so this is a
// no-op.
func (s *FileStore) Close() error { return nil }

// snapshotLocked writes the whole record map atomically. Caller holds s.mu.
func (s *FileStore) snapshotLocked() error {
	// MarshalIndent over a map sorts keys alphabetically (Go guarantee),
	// giving deterministic on-disk diffs.
	body, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("memory: marshal: %w", err)
	}
	return atomicWrite(s.path, body)
}

// sortRecords orders records deterministically: oldest first, ties broken by
// id. Shared by All and tests.
func sortRecords(rs []Record) {
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].CreatedMS != rs[j].CreatedMS {
			return rs[i].CreatedMS < rs[j].CreatedMS
		}
		return rs[i].ID < rs[j].ID
	})
}

// Scored is a record paired with its retrieval score.
type Scored struct {
	Record Record  `json:"record"`
	Score  float64 `json:"score"`
}

// Search ranks the usable records in rs against query and returns the top
// `limit` by score (descending). A record scores on keyword overlap between
// the query tokens and the record's subject+content+tags, weighted by
// confidence and recency. Records with zero keyword overlap are excluded, as
// are tombstoned, superseded, suspended, and expired records.
//
// Ranking is a pure function of (rs, query, limit, nowMS): given the same
// inputs it returns the same ordering, with ties broken by LastSeenMS
// (newer first) then id — so callers and tests get stable output.
//
// nowMS is the reference time for the recency factor; callers pass the
// current wall clock (tests pass a fixed value).
func Search(rs []Record, query string, limit int, nowMS int64) []Scored {
	qTokens := tokenize(query)
	out := make([]Scored, 0, len(rs))
	if len(qTokens) == 0 {
		return out
	}
	for _, r := range rs {
		if !r.Usable(nowMS) {
			continue
		}
		overlap := keywordOverlap(qTokens, r)
		if overlap == 0 {
			continue
		}
		conf := r.Confidence
		if conf <= 0 {
			conf = 0.5 // never zero-out a hit purely on missing confidence
		}
		score := float64(overlap) * (0.5 + conf) * recencyFactor(r.LastSeenMS, nowMS)
		out = append(out, Scored{Record: r, Score: score})
	}
	sortScored(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// keywordOverlap counts how many distinct query tokens appear anywhere in the
// record's searchable text (subject + content + tag keys/values — searchText,
// shared with the embedder so both signals see the same surface).
func keywordOverlap(qTokens []string, r Record) int {
	haystack := make(map[string]struct{})
	for _, t := range tokenize(searchText(r)) {
		haystack[t] = struct{}{}
	}
	n := 0
	seen := make(map[string]struct{}, len(qTokens))
	for _, t := range qTokens {
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		if _, ok := haystack[t]; ok {
			n++
		}
	}
	return n
}

// recencyFactor decays linearly-ish with age: 1.0 for "now", asymptoting
// toward (but never reaching) 0 for old records. A record never drops out of
// retrieval on recency alone — it only loses ranking weight.
func recencyFactor(lastSeenMS, nowMS int64) float64 {
	ageMS := nowMS - lastSeenMS
	if ageMS <= 0 {
		return 1.0
	}
	const dayMS = 24 * 60 * 60 * 1000
	ageDays := float64(ageMS) / float64(dayMS)
	return 1.0 / (1.0 + ageDays)
}

// tokenize lowercases s and splits on any non-alphanumeric rune, dropping
// empties and 1-character tokens (noise). Stable and dependency-free.
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := fields[:0]
	for _, f := range fields {
		if len(f) <= 1 {
			continue
		}
		out = append(out, f)
	}
	return out
}

// atomicWrite writes data to a temp file and renames it over the target.
// os.Rename replaces atomically on POSIX and on Windows (MoveFileEx). Mirrors
// kernel/state's helper (kept local to avoid coupling the two packages).
func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("memory: open temp %s: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("memory: write temp: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("memory: fsync temp: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("memory: close temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("memory: rename %s: %w", path, err)
	}
	return nil
}
