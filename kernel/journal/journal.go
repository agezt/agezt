// SPDX-License-Identifier: MIT

// Package journal is the append-only, BLAKE3-hash-chained event log.
//
// It is the audit/replay/revert truth (DECISIONS B0c): every meaningful
// kernel action lands here first, then the bus publishes
// (durable-before-publish). Segments rotate at a configurable byte boundary
// (default 64 MiB per DECISIONS D1). Sidecar indices are a future
// optimization; M0.5 verifies and iterates by sequential scan.
//
// Concurrency: Journal is safe for concurrent use from multiple goroutines;
// Append serializes writes under a mutex so the sequence counter and chain
// head stay coherent.
package journal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

// DefaultSegmentBytes is the default rotation threshold (DECISIONS D1).
const DefaultSegmentBytes int64 = 64 * 1024 * 1024

// segmentExt is the JSONL segment file extension.
const segmentExt = ".jsonl"

// segmentDigits is the zero-padded width of segment-index filenames.
const segmentDigits = 8

// Options tweak Journal behavior. Zero values pick sensible defaults.
type Options struct {
	// SegmentBytes is the rotation threshold in bytes. <=0 → default.
	SegmentBytes int64
	// Now is the time source used for event timestamps. nil → time.Now.
	Now func() time.Time
	// IDGen mints event IDs. nil → ulid.New.
	IDGen func() string
}

// Journal owns the on-disk event log under a single directory.
type Journal struct {
	dir      string
	segBytes int64
	now      func() time.Time
	idGen    func() string

	mu       sync.Mutex
	nextSeq  int64
	head     string // last event hash, GenesisHash for an empty chain
	curFile  *os.File
	curBytes int64
	curIndex int // current segment number (1-based)
}

// Open opens or creates a journal at dir. If dir contains existing segments,
// it scans them to recover the head sequence and hash. A chain break in any
// existing segment is reported as an error.
func Open(dir string, opt Options) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("journal: mkdir %s: %w", dir, err)
	}
	j := &Journal{
		dir:      dir,
		segBytes: opt.SegmentBytes,
		now:      opt.Now,
		idGen:    opt.IDGen,
		head:     event.GenesisHash,
	}
	if j.segBytes <= 0 {
		j.segBytes = DefaultSegmentBytes
	}
	if j.now == nil {
		j.now = time.Now
	}
	if j.idGen == nil {
		j.idGen = ulid.New
	}

	segs, err := listSegments(dir)
	if err != nil {
		return nil, err
	}
	if len(segs) == 0 {
		// Fresh journal: start at segment 1.
		j.curIndex = 1
		if err := j.openCurrent(false); err != nil {
			return nil, err
		}
		return j, nil
	}

	// Recover: scan every segment in order to find seq and head.
	for _, s := range segs {
		if err := j.scanSegment(s); err != nil {
			return nil, err
		}
	}

	// Decide where to append: last segment if under threshold, else next.
	last := segs[len(segs)-1]
	j.curIndex = last.idx
	info, err := os.Stat(last.path)
	if err != nil {
		return nil, fmt.Errorf("journal: stat last segment: %w", err)
	}
	if info.Size() >= j.segBytes {
		j.curIndex++
		j.curBytes = 0
		if err := j.openCurrent(false); err != nil {
			return nil, err
		}
	} else {
		// A crash mid-write can leave a torn (newline-less) fragment at the end of
		// the last segment. Readers/recovery discard it (scanCompleteLines), but the
		// next append uses O_APPEND and would write AFTER the fragment, gluing a new
		// record onto the partial one — producing a line nothing can decode and
		// wedging the journal permanently. Truncate to the end of the last complete
		// line so the next append begins exactly where the last committed record
		// ended (the invariant: append offset == end of last committed line).
		good, err := lastCompleteOffset(last.path)
		if err != nil {
			return nil, err
		}
		if good != info.Size() {
			if err := os.Truncate(last.path, good); err != nil {
				return nil, fmt.Errorf("journal: truncate torn tail of %s: %w", last.path, err)
			}
		}
		j.curBytes = good
		if err := j.openCurrent(true); err != nil {
			return nil, err
		}
	}
	return j, nil
}

// lastCompleteOffset returns the byte length of the newline-terminated prefix of
// the segment at path — i.e. the offset just past the last committed line. Any
// bytes after that are a torn final line (a crash mid-write) and are not a
// committed record. 0 means no complete line is present.
func lastCompleteOffset(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("journal: read %s: %w", path, err)
	}
	i := bytes.LastIndexByte(data, '\n')
	if i < 0 {
		return 0, nil
	}
	return int64(i + 1), nil
}

// Close flushes and closes the current segment.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.curFile == nil {
		return nil
	}
	err := j.curFile.Close()
	j.curFile = nil
	return err
}

// Head returns the latest assigned sequence (0 for an empty journal) and
// the latest event hash (GenesisHash for an empty journal).
func (j *Journal) Head() (seq int64, hash string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.nextSeq - 1, j.head
}

// Append assigns identity (ULID + monotonic seq + ts + prev_hash), computes
// the chain hash, writes the event to the current segment, fsyncs, rotates
// if needed, and returns the persisted event.
//
// The write is durable before the function returns — bus publication can
// safely follow (durable-before-publish, TASKS P0-BUS-03).
func (j *Journal) Append(spec event.Spec) (*event.Event, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	id := j.idGen()
	seq := j.nextSeq
	ts := j.now()
	prev := j.head

	e, err := event.New(spec, id, seq, ts, prev)
	if err != nil {
		return nil, err
	}

	line, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("journal: marshal event: %w", err)
	}
	line = append(line, '\n')

	if err := j.writeAndSync(line); err != nil {
		return nil, err
	}
	j.head = e.Hash
	j.nextSeq++
	return e, nil
}

// Range iterates every event in seq order, calling fn for each. Iteration
// stops at the first non-nil error returned by fn (returned to the caller)
// or at the first parse error in a segment. Use Verify for chain integrity.
func (j *Journal) Range(fn func(*event.Event) error) error {
	segs, err := listSegments(j.dir)
	if err != nil {
		return err
	}
	for _, s := range segs {
		f, err := os.Open(s.path)
		if err != nil {
			return fmt.Errorf("journal: open %s: %w", s.path, err)
		}
		sc := newLineScanner(f)
		for sc.Scan() {
			ev, err := event.Decode(sc.Bytes())
			if err != nil {
				f.Close()
				return fmt.Errorf("journal: decode in %s: %w", s.path, err)
			}
			if err := fn(ev); err != nil {
				f.Close()
				return err
			}
		}
		err = sc.Err()
		f.Close()
		if err != nil {
			return fmt.Errorf("journal: scan %s: %w", s.path, err)
		}
	}
	return nil
}

// Tail returns the last n events in seq order, reading from the newest segment
// backwards and stopping as soon as it has enough — so `journal tail` (and any
// "recent events" view) costs O(events read) ≈ the last segment, not O(total).
// For n <= 0 it returns nil. Fewer than n total events returns them all.
//
// Concurrency matches Range: no lock is held, so a Tail runs alongside Append
// (it reads complete, fsync'd lines). The current segment may gain events during
// the read; Tail reflects whatever was durably written when it reached EOF.
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
	sc := newLineScanner(f)
	for sc.Scan() {
		ev, err := event.Decode(sc.Bytes())
		if err != nil {
			return nil, fmt.Errorf("journal: decode in %s: %w", s.path, err)
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("journal: scan %s: %w", s.path, err)
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

	if err := os.MkdirAll(dir, 0o755); err != nil {
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
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
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
	next, err := os.OpenFile(j.segmentPath(j.curIndex+1), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
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
func (j *Journal) openCurrent(appendMode bool) error {
	flag := os.O_WRONLY | os.O_CREATE
	if appendMode {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_EXCL
	}
	path := j.segmentPath(j.curIndex)
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		return fmt.Errorf("journal: open %s: %w", path, err)
	}
	j.curFile = f
	if !appendMode {
		// A fresh segment was just created — persist its directory entry.
		_ = syncDir(j.dir)
	}
	return nil
}

// scanSegment scans a segment for recovery: updates nextSeq and head, fails
// on chain break.
func (j *Journal) scanSegment(s segment) error {
	f, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("journal: open %s: %w", s.path, err)
	}
	defer f.Close()

	sc := newLineScanner(f)
	for sc.Scan() {
		ev, err := event.Decode(sc.Bytes())
		if err != nil {
			return fmt.Errorf("journal: decode in %s: %w", s.path, err)
		}
		if ev.Seq != j.nextSeq {
			return fmt.Errorf("%w: %s: expected seq %d, got %d", ErrChainBreak, s.path, j.nextSeq, ev.Seq)
		}
		if ev.PrevHash != j.head {
			return fmt.Errorf("%w: %s: seq %d prev %s != head %s", ErrChainBreak, s.path, ev.Seq, ev.PrevHash, j.head)
		}
		if err := ev.VerifyHash(); err != nil {
			return fmt.Errorf("%w: %s: %w", ErrChainBreak, s.path, err)
		}
		j.head = ev.Hash
		j.nextSeq++
	}
	return sc.Err()
}

func (j *Journal) segmentPath(idx int) string {
	name := fmt.Sprintf("%0*d%s", segmentDigits, idx, segmentExt)
	return filepath.Join(j.dir, name)
}

// segment is a discovered segment file on disk.
type segment struct {
	idx  int
	path string
}

func listSegments(dir string) ([]segment, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("journal: readdir %s: %w", dir, err)
	}
	var segs []segment
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, segmentExt) {
			continue
		}
		base := strings.TrimSuffix(name, segmentExt)
		idx, err := strconv.Atoi(base)
		if err != nil {
			// Non-numeric segment names are foreign; ignore.
			continue
		}
		segs = append(segs, segment{idx: idx, path: filepath.Join(dir, name)})
	}
	sort.Slice(segs, func(i, k int) bool { return segs[i].idx < segs[k].idx })
	return segs, nil
}

// newLineScanner returns a bufio.Scanner sized for events larger than the
// default token buffer (which is too small for 32-KiB-payload events,
// DECISIONS B5).
func newLineScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	const max = 1 << 20 // 1 MiB per line — generous headroom for attachments
	sc.Buffer(make([]byte, 64*1024), max)
	sc.Split(scanCompleteLines)
	return sc
}

// scanCompleteLines is like bufio.ScanLines but DISCARDS a trailing unterminated
// line at EOF instead of yielding it as a token. Every durably-written journal
// line ends in '\n' (writeAndSync appends it), so the only line ever missing its
// newline is an in-flight append observed by a concurrent Range/Tail, or a crash
// mid-write — neither is a committed record. Discarding it makes every reader
// (Range/Tail/Verify) and recovery tolerant of a torn final line rather than
// failing to JSON-decode a partial record. A torn line can only ever be the LAST
// line of the current segment (appends are serialized), so a corrupt MIDDLE line
// still surfaces as a decode error (it precedes a real '\n').
func scanCompleteLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		// Remaining bytes carry no newline → an unterminated trailing line.
		// Advance past it and emit nothing (do not surface it as a record).
		return len(data), nil, nil
	}
	return 0, nil, nil // need more data to complete a line
}
