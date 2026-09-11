// SPDX-License-Identifier: MIT

// Journal core: Open + lastCompleteOffset + Close + Head + Append + Range.
// Code extracted from journal.go during the Day-60 god-file split. Public API unchanged.
package journal


import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
	"time"
	"os"
	"sync"
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

// Permissions for the journal directory and its segments (EXPOSE-001,
// 2026-08-12). The journal holds full prompts, raw tool arguments, tool outputs
// and captured HTTP Authorization / Set-Cookie headers — the most sensitive
// at-rest data the daemon owns — and it shipped world-readable (0644 segments in
// a 0755 directory) while the vault, artifacts, auth tokens and datalake all
// used 0600/0700. Any other local user could read the entire history with no
// credential.
//
// The journal is append-only with no purge path, so a redaction miss is
// permanent; that makes the at-rest mode the cheapest defence available and the
// one worth getting right.
const (
	journalDirPerm     = 0o700
	journalSegmentPerm = 0o600
)

// Open opens or creates a journal at dir. If dir contains existing segments,
// it scans them to recover the head sequence and hash. A chain break in any
// existing segment is reported as an error.
func Open(dir string, opt Options) (*Journal, error) {
	if err := os.MkdirAll(dir, journalDirPerm); err != nil {
		return nil, fmt.Errorf("journal: mkdir %s: %w", dir, err)
	}
	// MkdirAll is a no-op on an existing directory, so a journal created before
	// this change would keep its 0755 forever. Tighten in place: without this,
	// the constants above only protect fresh installs and every existing one
	// stays exposed.
	//
	// Best-effort on purpose. My first version returned the error, which would
	// have refused to open the journal — and therefore failed daemon boot — on
	// any filesystem where chmod cannot succeed (some network mounts, some
	// container ownership setups). Refusing to start over a permissions nicety
	// inverts the cost: the operator loses the daemon entirely instead of losing
	// one hardening measure, and it contradicts the boot-resilience rule that a
	// recoverable mismatch warns and degrades rather than hard-failing.
	_ = os.Chmod(dir, journalDirPerm)
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
	// Tighten segments written before this change. The directory mode above
	// already stops a fresh traversal, but a segment path that leaked (a backup,
	// a support bundle, a symlink) would still be world-readable, and the
	// journal has no purge path — history written yesterday is history forever.
	// Best-effort per file: a chmod failure must not make an otherwise healthy
	// journal unopenable.
	for _, s := range segs {
		_ = os.Chmod(s.path, journalSegmentPerm)
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
		err = rangeCompleteLines(s.path, f, func(line []byte) error {
			ev, err := event.Decode(line)
			if err != nil {
				return fmt.Errorf("journal: decode in %s: %w", s.path, err)
			}
			if err := fn(ev); err != nil {
				return err
			}
			return nil
		})
		f.Close()
		if err != nil {
			return err
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