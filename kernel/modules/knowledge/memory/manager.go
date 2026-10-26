// SPDX-License-Identifier: MIT

package memory

// Provenance: Memory Manager: Manager struct + NewManager + Remember (the journaling
//             boundary that wraps a Store + bus). Code extracted from manager.go
//             during the Day-44 god-file split. Public API unchanged.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/event"
)

// Manager wraps a Store with the kernel bus so every mutation is journaled
// (durable-before-publish) and carries the originating run's correlation_id.
// This is the journaling boundary — the Store itself stays pure. It mirrors
// how kernel/runtime publishes kernel.halt/resume rather than the state store
// doing its own journaling.
//
// The manager holds PORTS, not implementations (W3.57): the store is the
// host's file, the publisher is the kernel's, and the structured-output
// generator is the host's seam. The module names none of them.
type Manager struct {
	store memoryapi.Store
	// pub journals every mutation (durable-before-publish). It is the
	// contract's publisher port: the module never names the bus, because the
	// bus answers with the durable envelope and the envelope carries the hash
	// chain the contract layer may not name.
	pub memoryapi.Publisher
	// gen is the structured-output path the distillers use. It is injected:
	// the module that owns it is not the memory module's to name.
	gen memoryapi.Generator
	// now is the clock, injectable for deterministic tests. Defaults to
	// time.Now when constructed via NewManager.
	now func() time.Time
	// mu serialises the read-modify-write mutators (Remember/Forget/Supersede) so
	// two concurrent writers — e.g. the agent loop and the auto-distiller both
	// remembering a fact, or a reinforce racing a forget — cannot interleave their
	// Get→Put pairs and lose an update (M421). The underlying Store guards each call
	// individually but not the pair.
	mu sync.Mutex
	// embMu guards the optional provider embedder + its vector cache (M884).
	// Separate from mu: embedding work must not block writers.
	embMu    sync.Mutex
	embedder memoryapi.Embedder
	embCache map[string][]float32 // record ID → vector; content-addressing makes it immutable
}

// NewManager wires a Store to a publisher and a structured-output generator.
// The publisher may be nil in tests that only exercise the store-facing
// behaviour, but production callers always pass the kernel's publisher so
// mutations are auditable. The generator may be nil too: the distillers then
// report that structured output is unavailable rather than pretending they
// distilled something.
func NewManager(deps memoryapi.Deps) *Manager {
	return &Manager{store: deps.Store, pub: deps.Publisher, gen: deps.Generator, now: time.Now}
}

// The manager answers the CONTRACT's ports (W3.57): the compile-time
// assertion is the proof that the module's face and the contract's port are
// the same shape — a second shape would mean a second set of fakes.
var (
	_ memoryapi.Manager = (*Manager)(nil)
	_ memoryapi.Reader  = (*Manager)(nil)
	_ memoryapi.Writer  = (*Manager)(nil)
)

// generate drives the injected structured-output path.
//
// The path lives in another module, so it is INJECTED (W3.57): a module may
// not name another module. A host that injected none gets a clear refusal —
// not a panic, and not a silent no-op.
func (m *Manager) generate(ctx context.Context, provider llm.Provider, req llm.CompletionRequest, schema json.RawMessage, out any) (llm.Usage, error) {
	if m.gen == nil {
		return llm.Usage{}, errors.New("memory: no structured-output generator was injected")
	}
	return m.gen(ctx, provider, req, schema, out)
}

// RememberSpec is the input to Remember.
//
// The row lives in the CONTRACT (W3.57) — the alias itself lives in
// memory.go, with the other rows.

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
	if !spec.Force && shouldFilterSpec(spec) {
		if d := assessSpec(spec); !d.Keep {
			return Record{}, false, fmt.Errorf("memory: low-value record rejected (%s)", d.Reason)
		}
	}
	evidence := normalizeEvidence(spec.Evidence, spec.Tags, t)
	halfLifeMS := spec.HalfLifeMS
	if halfLifeMS <= 0 {
		halfLifeMS = defaultHalfLifeMS(t, evidence)
	}
	nowMS := m.now().UnixMilli()
	// Scope participates in identity (M915): two agents privately noting the
	// same content get two records, instead of the second write reinforcing the
	// first and flipping its scope tag (which would hide the note from its
	// original author). Shared writes hash exactly as before.
	id := ScopedID(t, spec.Subject, spec.Content, scopeOf(spec.Tags))

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
		Evidence:   evidence,
		HalfLifeMS: halfLifeMS,
		CreatedMS:  nowMS,
		LastSeenMS: nowMS,
		AddedBy:    spec.Actor,
		UpdatedBy:  spec.Actor,
	}
	action := "create"
	if found {
		// Reinforce: keep original creation time, refresh recency, and
		// strengthen confidence toward 1.0. Revive if it was tombstoned.
		rec.CreatedMS = existing.CreatedMS
		rec.SourceEvent = existing.SourceEvent
		rec.Confidence = clampConf(existing.Confidence + 0.1)
		if spec.Evidence == "" {
			rec.Evidence = existing.Evidence
		}
		if spec.HalfLifeMS <= 0 {
			rec.HalfLifeMS = existing.HalfLifeMS
		}
		// Provenance: AddedBy is first-writer-wins (preserve the original author,
		// like SourceEvent); UpdatedBy reflects this latest write. A legacy record
		// with no AddedBy adopts this writer as its author.
		if existing.AddedBy != "" {
			rec.AddedBy = existing.AddedBy
		}
		if spec.Actor == "" {
			rec.UpdatedBy = existing.UpdatedBy
		}
		if existing.Tags != nil && rec.Tags == nil {
			rec.Tags = existing.Tags
		}
		// A successful re-observation/reconstruction makes a suspended record
		// usable again without erasing its audit history from the journal.
		rec.SuspendedMS = 0
		rec.SuspendedReason = ""
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
	payload := map[string]any{
		"action":     action,
		"id":         id,
		"type":       string(t),
		"subject":    spec.Subject,
		"chars":      len(spec.Content),
		"confidence": rec.Confidence,
	}
	if rec.Evidence != "" {
		payload["evidence"] = string(rec.Evidence)
	}
	if rec.HalfLifeMS > 0 {
		payload["half_life_ms"] = rec.HalfLifeMS
	}
	if rec.UpdatedBy != "" {
		payload["actor"] = rec.UpdatedBy // who wrote this (M851)
	}
	eventID := m.publish(event.KindMemoryWritten, corr, payload)
	if eventID != "" && rec.SourceEvent == "" {
		rec.SourceEvent = eventID
	}
	if err := m.store.Put(rec); err != nil {
		return Record{}, false, err
	}
	return rec, !found, nil
}
