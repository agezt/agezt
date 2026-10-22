// SPDX-License-Identifier: MIT

package runtime

// The artifact collector's knowledge of what the journal still names.
//
// A tool output that is too large to inline is offloaded into the content
// addressed artifact store, and the journaled tool.result carries the blob's
// `raw_ref` instead of the bytes (SPEC-04 §3.6). The journal is append-only
// with FULL retention — segments rotate but are never deleted — so every ref it
// ever recorded is still named there.
//
// That makes the journal the owner of those bytes: a blob the journal still
// names must not be garbage-collected, or the raw_ref becomes a dangling
// promise (`agt artifact_get <ref>` fails and the run's output is lost while
// the journal still claims the bytes are there).
//
// The module cannot read the journal itself (a module sees another module's
// api package and nothing else — P5), so it asks: the artifact index takes a
// RefGuard and the host fills it with this implementation.

import (
	"encoding/json"
	"errors"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// errRefsFound stops the journal scan as soon as every candidate ref is
// accounted for — the scan is a full pass over the event log, and a candidate
// that was journaled long ago is usually found early.
var errRefsFound = errors.New("runtime: all candidate refs found")

// journalRefGuard answers "does the journal still name this blob?" from the
// journal's own events.
type journalRefGuard struct {
	j *journal.Journal
}

// ReferencedRefs returns the subset of candidates whose ref is named by a
// journaled tool.result. The journal is asked once per operation (the module
// passes every candidate it is about to touch), so one pass over the event log
// answers the whole batch.
func (g *journalRefGuard) ReferencedRefs(candidates []string) map[string]bool {
	out := map[string]bool{}
	if g == nil || g.j == nil || len(candidates) == 0 {
		return out
	}
	want := make(map[string]struct{}, len(candidates))
	for _, c := range candidates {
		if c != "" {
			want[c] = struct{}{}
		}
	}
	if len(want) == 0 {
		return out
	}
	_ = g.j.Range(func(ev *event.Event) error {
		if ev.Kind != event.KindToolResult {
			return nil
		}
		var payload struct {
			RawRef string `json:"raw_ref"`
		}
		if json.Unmarshal(ev.Payload, &payload) != nil || payload.RawRef == "" {
			return nil
		}
		if _, ok := want[payload.RawRef]; !ok {
			return nil
		}
		out[payload.RawRef] = true
		if len(out) == len(want) {
			return errRefsFound
		}
		return nil
	})
	return out
}
