// SPDX-License-Identifier: MIT

// Memory Manager: Manager struct + NewManager + Remember (the journaling boundary that wraps a Store + bus).
// Code extracted from manager.go during the Day-44 god-file split. Public API unchanged.
package memory


import (
	"fmt"
	"strings"
	"sync"
	"time"

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
	// embMu guards the optional provider embedder + its vector cache (M884).
	// Separate from mu: embedding work must not block writers.
	embMu    sync.Mutex
	embedder Embedder
	embCache map[string][]float32 // record ID → vector; content-addressing makes it immutable
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
	Evidence   Evidence
	HalfLifeMS int64
	// Actor records WHO is writing (M851): the acting agent's slug, or
	// "operator"/"distill" for non-agent writes. Stored as AddedBy (first writer)
	// and UpdatedBy (latest writer) on the record. Empty leaves provenance unset.
	Actor string
	// Force bypasses long-term retention filtering. Operator/control-plane writes
	// set this because they are explicit curation; automatic and agent writes do
	// not.
	Force bool
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
	ev := m.publish(event.KindMemoryWritten, corr, payload)
	if ev != nil && rec.SourceEvent == "" {
		rec.SourceEvent = ev.ID
	}
	if err := m.store.Put(rec); err != nil {
		return Record{}, false, err
	}
	return rec, !found, nil
}
