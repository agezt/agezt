// SPDX-License-Identifier: MIT

// journal_open.go owns the cold-start path: the Open
// constructor that scans existing segments to recover the
// head sequence + hash and tightens the on-disk mode in place
// from 0755/0644 to 0700/0600 (EXPOSE-001, 2026-08-12). The
// runtime surface (Close / Head / Append / Range) + the
// Journal type live in journal.go.
package journal

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
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
