// SPDX-License-Identifier: MIT

package journal

import (
	"fmt"
	"strconv"
	"strings"
)

// Cursor pagination over journal-derived lists.
//
// A cursor is an opaque `<ms>:<seq>` token identifying a boundary row in a
// list sorted DESCENDING by (ms, seq) — newest first. `ms` is a wall-clock
// millisecond (bus resolution is 1ms, so back-to-back rows can collide) and
// `seq` is the journal sequence number, used as the tie-break so a run that
// arrived later still sorts first within the same millisecond.
//
// The token round-trips through Go's fmt and through the browser's
// URLSearchParams (the colon is percent-encoded on the wire; DecodeCursor
// tolerates the raw form here — the HTTP layer decodes %3A before it reaches
// the handler).
//
// These helpers were extracted from kernel/controlplane/runs.go (the
// /api/runs pager) so every journal-backed list endpoint (runs, agents,
// inbox/board/memory, and the *_log endpoints) shares one cursor
// implementation instead of copying the parse/encode/filter logic. See
// docs/REFACTOR-A1-CONTROLPLANE-PLAN.md (Phase 1) and
// docs/REFACTOR-A2-LOG-PAGINATION-PLAN.md.

// EncodeCursor produces the opaque token for the boundary row at (ms, seq).
// The all-zero pair encodes to the empty string, which DecodeCursor reads back
// as "absent" — callers use the empty string to mean "no next page".
func EncodeCursor(ms, seq int64) string {
	if ms == 0 && seq == 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d", ms, seq)
}

// DecodeCursor parses an opaque `<ms>:<seq>` token.
//
// It returns ok=false (and zero ms/seq) when the token is absent, malformed,
// or negative. Callers MUST treat !ok as "no cursor — return the newest page"
// rather than erroring: a forged/aged-out/hand-crafted cursor gracefully falls
// back to the first page, which is closer to "the user got their list" than
// "the user got an error".
//
// The value is accepted as `any` because control-plane args arrive as decoded
// JSON (a string here); a non-string value reads as absent.
func DecodeCursor(raw any) (ms, seq int64, ok bool) {
	s, _ := raw.(string)
	if s == "" {
		return 0, 0, false
	}
	i := strings.IndexByte(s, ':')
	// The colon must be interior: neither the first nor the last byte, so both
	// halves are non-empty.
	if i <= 0 || i >= len(s)-1 {
		return 0, 0, false
	}
	parsedMS, err := strconv.ParseInt(s[:i], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	parsedSeq, err := strconv.ParseInt(s[i+1:], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	// A negative ms or seq is not a valid journal position — treat as malformed.
	if parsedMS < 0 || parsedSeq < 0 {
		return 0, 0, false
	}
	return parsedMS, parsedSeq, true
}

// KeepBeforeCursor reports whether a row at (rowMS, rowSeq) belongs on the
// page AFTER the cursor at (cursorMS, cursorSeq) — i.e. whether the row is
// STRICTLY OLDER than the cursor in the descending (ms, seq) sort.
//
// The equality clause is critical: the cursor encodes the OLDEST row already
// emitted on the previous page, so a row equal to the cursor is one the client
// already has and must be dropped, or it re-appears at the top of the next page.
//
// Usage in a handler (rows already sorted DESC by (ms, seq)):
//
//	cursorMS, cursorSeq, ok := journal.DecodeCursor(args["cursor"])
//	if ok {
//	    kept := rows[:0]
//	    for _, r := range rows {
//	        if journal.KeepBeforeCursor(r.MS, r.Seq, cursorMS, cursorSeq) {
//	            kept = append(kept, r)
//	        }
//	    }
//	    rows = kept
//	}
func KeepBeforeCursor(rowMS, rowSeq, cursorMS, cursorSeq int64) bool {
	if rowMS > cursorMS {
		return false
	}
	if rowMS == cursorMS && rowSeq >= cursorSeq {
		return false
	}
	return true
}

// NextCursor returns the token a handler should advertise for the following
// page, given the rows it is about to emit (already sorted DESC and already
// truncated to the page limit) and that limit.
//
// It encodes the LAST (oldest) emitted row so the next page's KeepBeforeCursor
// filter skips exactly the rows already returned — encoding the first (newest)
// row would only skip a single row and re-emit the rest.
//
// It returns "" (no next page) unless the page is full (len == limit); a short
// page is terminal, so advertising a cursor would invite a pointless empty
// fetch. rows must expose its boundary via the caller-supplied last (ms, seq).
func NextCursor(lastMS, lastSeq int64, emitted, limit int) string {
	if emitted == 0 || emitted < limit {
		return ""
	}
	return EncodeCursor(lastMS, lastSeq)
}
