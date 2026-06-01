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
		j.curBytes = info.Size()
		if err := j.openCurrent(true); err != nil {
			return nil, err
		}
	}
	return j, nil
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

// writeAndSync writes the line to the current segment, fsyncs, and rotates
// if the resulting size meets/exceeds segBytes. Caller holds j.mu.
func (j *Journal) writeAndSync(line []byte) error {
	if _, err := j.curFile.Write(line); err != nil {
		return fmt.Errorf("journal: write: %w", err)
	}
	if err := j.curFile.Sync(); err != nil {
		return fmt.Errorf("journal: fsync: %w", err)
	}
	j.curBytes += int64(len(line))

	if j.curBytes >= j.segBytes {
		if err := j.curFile.Close(); err != nil {
			return fmt.Errorf("journal: close rotating segment: %w", err)
		}
		j.curIndex++
		j.curBytes = 0
		if err := j.openCurrent(false); err != nil {
			return err
		}
	}
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
	return sc
}
