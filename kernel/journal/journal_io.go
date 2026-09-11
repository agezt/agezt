// SPDX-License-Identifier: MIT

// Journal I/O: Tail + readSegment + Restore + Verify + writeAndSync + rotate.
// Code extracted from journal.go during the Day-60 god-file split. Public API unchanged.
package journal


import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/event"
	"io"
	"os"
	"path/filepath"
)


func (j *Journal) Tail(n int) ([]*event.Event, error) {
	if n <= 0 {
		return nil, nil
	}
	segs, err := listSegments(j.dir)
	if err != nil {
		return nil, err
	}
	// Read segments newest→oldest, prepending each so the result stays in seq
	// order, until we've gathered at least n events (or run out of segments).
	var collected []*event.Event
	for i := len(segs) - 1; i >= 0; i-- {
		evs, err := readSegment(segs[i])
		if err != nil {
			return nil, err
		}
		collected = append(evs, collected...)
		if len(collected) >= n {
			break
		}
	}
	if len(collected) > n {
		collected = collected[len(collected)-n:]
	}
	return collected, nil
}

// readSegment scans one segment file fully into a slice of events, in order.
func readSegment(s segment) ([]*event.Event, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("journal: open %s: %w", s.path, err)
	}
	defer f.Close()
	var out []*event.Event
	err = rangeCompleteLines(s.path, f, func(line []byte) error {
		ev, err := event.Decode(line)
		if err != nil {
			return fmt.Errorf("journal: decode in %s: %w", s.path, err)
		}
		out = append(out, ev)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ErrChainBreak is returned by Verify and recovery scans when the chain is
// inconsistent (bad hash, missing seq, or non-monotonic seq).
var ErrChainBreak = errors.New("journal: chain break")

// ErrNotEmpty is returned by Restore when the target directory already holds
// journal segments — restore never clobbers an existing chain.
var ErrNotEmpty = errors.New("journal: target already has segments")

// ErrNotFullExport is returned by Restore when the event slice does not begin
// at seq 0 with prev_hash == GenesisHash. A windowed export (`--since`) starts
// mid-chain and cannot seed a bootable journal.
var ErrNotFullExport = errors.New("journal: restore needs a full export (must start at seq 0 from genesis)")

// Restore seeds an EMPTY journal directory from a verified, genesis-anchored
// event slice — the disaster-recovery / migration read-back of an export
// bundle (M102). It is deliberately strict and non-destructive:
//
//   - the directory must contain no existing segments (else ErrNotEmpty);
//   - the slice must start at seq 0 with prev_hash == GenesisHash and chain-
//     verify end-to-end (else ErrNotFullExport / ErrChainBreak), so the
//     resulting journal boots cleanly through the same scan Open() runs.
//
// On success it writes every event verbatim (compact, one per line) as the
// initial segment and returns the restored head seq + hash. On any failure it
// writes nothing (validation happens before the first byte hits disk).
func Restore(dir string, events []*event.Event) (headSeq int64, headHash string, err error) {
	if len(events) == 0 {
		return 0, "", fmt.Errorf("%w: empty event set", ErrNotFullExport)
	}
	if events[0].Seq != 0 || events[0].PrevHash != event.GenesisHash {
		return 0, "", ErrNotFullExport
	}

	// Full chain verification BEFORE touching disk: seq monotonic from 0,
	// prev-hash continuity, and each recomputed hash matches. Mirrors Verify
	// (which needs an open journal) for an in-memory slice.
	prev := event.GenesisHash
	var expectedSeq int64
	lines := make([][]byte, 0, len(events))
	for _, e := range events {
		if e.Seq != expectedSeq {
			return 0, "", fmt.Errorf("%w: expected seq %d, got %d (id=%s)", ErrChainBreak, expectedSeq, e.Seq, e.ID)
		}
		if e.PrevHash != prev {
			return 0, "", fmt.Errorf("%w: seq %d prev_hash %s != actual %s", ErrChainBreak, e.Seq, e.PrevHash, prev)
		}
		if verr := e.VerifyHash(); verr != nil {
			return 0, "", fmt.Errorf("%w: seq %d: %w", ErrChainBreak, e.Seq, verr)
		}
		// Re-marshal compactly so the segment line matches Append's format
		// regardless of how the bundle stored (and possibly re-indented) it.
		line, merr := json.Marshal(e)
		if merr != nil {
			return 0, "", fmt.Errorf("journal: marshal event seq %d: %w", e.Seq, merr)
		}
		lines = append(lines, append(line, '\n'))
		prev = e.Hash
		expectedSeq++
	}

	if err := os.MkdirAll(dir, journalDirPerm); err != nil {
		return 0, "", fmt.Errorf("journal: mkdir %s: %w", dir, err)
	}
	existing, err := listSegments(dir)
	if err != nil {
		return 0, "", err
	}
	if len(existing) > 0 {
		return 0, "", ErrNotEmpty
	}

	path := filepath.Join(dir, fmt.Sprintf("%0*d%s", segmentDigits, 1, segmentExt))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, journalSegmentPerm)
	if err != nil {
		return 0, "", fmt.Errorf("journal: create segment %s: %w", path, err)
	}
	for _, line := range lines {
		if _, werr := f.Write(line); werr != nil {
			f.Close()
			os.Remove(path)
			return 0, "", fmt.Errorf("journal: write segment: %w", werr)
		}
	}
	if serr := f.Sync(); serr != nil {
		f.Close()
		os.Remove(path)
		return 0, "", fmt.Errorf("journal: fsync segment: %w", serr)
	}
	if cerr := f.Close(); cerr != nil {
		os.Remove(path)
		return 0, "", fmt.Errorf("journal: close segment: %w", cerr)
	}
	_ = syncDir(dir) // persist the new segment's directory entry (power-loss safety)

	last := events[len(events)-1]
	return last.Seq, last.Hash, nil
}

// Verify replays every event, recomputes its hash, and confirms each one
// chains from its predecessor. Returns nil iff the entire chain is intact.
// On break, returns ErrChainBreak wrapped with the offending seq.
func (j *Journal) Verify() error {
	prev := event.GenesisHash
	var expectedSeq int64
	return j.Range(func(e *event.Event) error {
		if e.Seq != expectedSeq {
			return fmt.Errorf("%w: expected seq %d, got %d (id=%s)", ErrChainBreak, expectedSeq, e.Seq, e.ID)
		}
		if e.PrevHash != prev {
			return fmt.Errorf("%w: seq %d prev_hash %s != actual %s", ErrChainBreak, e.Seq, e.PrevHash, prev)
		}
		if err := e.VerifyHash(); err != nil {
			return fmt.Errorf("%w: seq %d: %w", ErrChainBreak, e.Seq, err)
		}
		prev = e.Hash
		expectedSeq++
		return nil
	})
}

// ----- internal helpers -----

// fsync is indirected so tests can simulate an fsync failure. It is the only
// path that flushes a just-written line to stable storage.
var fsync = (*os.File).Sync

// syncDir fsyncs a directory so a newly created segment's directory entry is
// durable, not just the file content — otherwise a freshly created/rotated segment
// (and its durable-before-publish records) can vanish on power loss even though the
// file was fsync'd. Indirected for tests. Best-effort at the call sites: a dir
// fsync can legitimately fail on some platforms (e.g. a directory handle on
// Windows), and that must not fail segment creation on the dev OS; the guarantee it
// adds is for the Linux deploy target.
var syncDir = func(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// writeAndSync writes the line to the current segment, fsyncs, and rotates
// if the resulting size meets/exceeds segBytes. Caller holds j.mu.
func (j *Journal) writeAndSync(line []byte) error {
	if _, err := j.curFile.Write(line); err != nil {
		return fmt.Errorf("journal: write: %w", err)
	}
	if err := fsync(j.curFile); err != nil {
		// The line is now in the file, but Append advances seq/head only after we
		// return nil — so it leaves nextSeq pointing at this seq. If we left the
		// un-synced line in place, the NEXT append would reuse the same seq and the
		// segment would hold two lines with that seq, tripping ErrChainBreak on the
		// next Open (a permanent boot wedge). Truncate the un-synced line back to the
		// last committed size so the file matches the in-memory chain; the caller
		// treats the append as failed (fail-closed). curBytes is the committed size
		// (it advances only after a successful sync) and the file is O_APPEND, so the
		// next write resumes exactly at curBytes. Seek too: on platforms where
		// O_APPEND is emulated (Windows) the handle's offset is left past the
		// truncation point, so the next write would land beyond curBytes and the OS
		// would zero-fill the gap — corrupting the segment. Seeking back keeps the
		// resume offset consistent on every platform.
		_ = j.curFile.Truncate(j.curBytes)
		_, _ = j.curFile.Seek(j.curBytes, io.SeekStart)
		return fmt.Errorf("journal: fsync: %w", err)
	}
	j.curBytes += int64(len(line))

	// The event is now durable. Rotation is housekeeping for the NEXT write —
	// a rotation failure must NOT fail this (already-committed) append, and must
	// not wedge the journal. rotate() is atomic (opens the next segment before
	// swapping), so on failure the current segment stays live and usable; it's
	// just left slightly oversized and the next append retries rotation.
	if j.curBytes >= j.segBytes {
		_ = j.rotate()
	}
	return nil
}

// rotate switches the active segment to the next index. It opens the new segment
// BEFORE swapping, so a failed open leaves the current (oversized) segment intact
// and the journal fully usable — never wedged with a closed handle (the prior
// close-then-open order could strand j.curFile on a closed file). Caller holds
// j.mu. A non-nil return leaves all state unchanged.
func (j *Journal) rotate() error {
	next, err := os.OpenFile(j.segmentPath(j.curIndex+1), os.O_WRONLY|os.O_CREATE|os.O_EXCL, journalSegmentPerm)
	if err != nil {
		return fmt.Errorf("journal: open next segment: %w", err)
	}
	old := j.curFile
	j.curFile = next
	j.curIndex++
	j.curBytes = 0
	_ = old.Close()    // best-effort: the new segment is already the live one
	_ = syncDir(j.dir) // persist the new segment's directory entry (power-loss safety)
	return nil
}

// openCurrent opens (or creates) the segment file for j.curIndex. If append
// is true, opens for append; otherwise creates fresh (must not exist).