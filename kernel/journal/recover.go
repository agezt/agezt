// SPDX-License-Identifier: MIT

package journal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

// QuarantineMarker is inserted between a segment's name and the quarantine
// timestamp. Quarantined files keep their bytes for forensics; because their
// names no longer end in the segment extension, no reader ever treats them as
// part of the live chain.
const QuarantineMarker = ".quarantined-"

// CorruptionError is a line in the middle of the journal that does not decode
// or does not continue the hash chain: disk damage, a manual edit, tampering.
type CorruptionError struct {
	Path string // segment holding the bad line
	Seq  int64  // the seq the chain expected at that line
	Err  error
}

func (e *CorruptionError) Error() string {
	return fmt.Sprintf("journal: corrupt record in %s (expected seq %d): %v", e.Path, e.Seq, e.Err)
}

func (e *CorruptionError) Unwrap() error { return e.Err }

// Recovery describes a quarantine performed by Open (owner decision 5.6,
// architecture/21): the chain was cut at the first bad record, everything from
// there moved aside, and the journal resumed from the last verified event.
type Recovery struct {
	// BreakSeq is the seq the chain expected at the first bad record; the live
	// chain resumes at this seq with the journal.recovered event.
	BreakSeq int64 `json:"break_seq"`
	// Reason is the corruption that triggered the quarantine.
	Reason string `json:"reason"`
	// Quarantined are the files holding the moved-aside bytes (base names, in
	// the journal directory).
	Quarantined []string `json:"quarantined"`
	// Bytes is the total size of the quarantined data.
	Bytes int64 `json:"bytes"`
}

// Recovery reports the quarantine Open performed, or nil when the journal
// opened clean.
func (j *Journal) Recovery() *Recovery { return j.recovery }

// quarantineFrom moves the chain's corrupt suffix aside: the bytes of
// segs[0] after offset good (the first bad record onward) and every later
// segment whole. Nothing is deleted: the tail is copied out and fsynced
// before the segment is truncated, and later segments are renamed.
func quarantineFrom(dir string, segs []segment, good int64, stamp time.Time) ([]string, int64, error) {
	suffix := QuarantineMarker + stamp.UTC().Format("20060102T150405Z")
	var names []string
	var total int64

	broken := segs[0]
	info, err := os.Stat(broken.path)
	if err != nil {
		return nil, 0, err
	}
	if info.Size() > good {
		tailName := filepath.Base(broken.path) + suffix
		n, err := copyTail(broken.path, filepath.Join(dir, tailName), good)
		if err != nil {
			return nil, 0, fmt.Errorf("quarantine tail of %s: %w", broken.path, err)
		}
		if err := os.Truncate(broken.path, good); err != nil {
			return nil, 0, fmt.Errorf("truncate %s: %w", broken.path, err)
		}
		names, total = append(names, tailName), total+n
	}
	for _, s := range segs[1:] {
		name := filepath.Base(s.path) + suffix
		info, err := os.Stat(s.path)
		if err != nil {
			return names, total, err
		}
		if err := os.Rename(s.path, filepath.Join(dir, name)); err != nil {
			return names, total, fmt.Errorf("quarantine %s: %w", s.path, err)
		}
		names, total = append(names, name), total+info.Size()
	}
	_ = syncDir(dir)
	return names, total, nil
}

// copyTail copies src's bytes from offset to EOF into a new file dst, fsynced.
func copyTail(src, dst string, offset int64) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	if _, err := in.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, journalSegmentPerm)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return n, err
}

// recordRecovery journals the quarantine as the first event of the resumed
// chain, so the gap is itself part of the audit trail.
func (j *Journal) recordRecovery() error {
	r := j.recovery
	_, err := j.Append(event.Spec{
		Subject: "journal.recovered",
		Kind:    event.KindJournalRecovered,
		Actor:   "journal",
		Payload: map[string]any{
			"break_seq":         r.BreakSeq,
			"reason":            r.Reason,
			"quarantined":       r.Quarantined,
			"quarantined_bytes": r.Bytes,
		},
	})
	return err
}

func isCorruption(err error) bool {
	var ce *CorruptionError
	return errors.As(err, &ce)
}
