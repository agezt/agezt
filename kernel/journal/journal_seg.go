// SPDX-License-Identifier: MIT

package journal

// Provenance: Journal segments: openCurrent + scanSegment + segmentPath +
//             listSegments + rangeCompleteLines. Code extracted from journal.go
//             during the Day-60 god-file split. Public API unchanged.

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/event"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func (j *Journal) openCurrent(appendMode bool) error {
	flag := os.O_WRONLY | os.O_CREATE
	if appendMode {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_EXCL
	}
	path := j.segmentPath(j.curIndex)
	f, err := os.OpenFile(path, flag, journalSegmentPerm)
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

// scanSegment scans a segment for recovery: updates nextSeq and head. It
// returns the byte offset just past the last line that verified. A line that
// does not decode or does not continue the chain is returned as a
// *CorruptionError (the caller decides whether to quarantine); a read failure
// is returned as-is.
func (j *Journal) scanSegment(s segment) (good int64, err error) {
	f, err := os.Open(s.path)
	if err != nil {
		return 0, fmt.Errorf("journal: open %s: %w", s.path, err)
	}
	defer f.Close()

	err = rangeCompleteLines(s.path, f, func(line []byte) error {
		ev, err := event.Decode(line)
		if err != nil {
			return &CorruptionError{Path: s.path, Seq: j.nextSeq, Err: fmt.Errorf("decode: %w", err)}
		}
		if ev.Seq != j.nextSeq {
			return &CorruptionError{Path: s.path, Seq: j.nextSeq, Err: fmt.Errorf("%w: expected seq %d, got %d", ErrChainBreak, j.nextSeq, ev.Seq)}
		}
		if ev.PrevHash != j.head {
			return &CorruptionError{Path: s.path, Seq: j.nextSeq, Err: fmt.Errorf("%w: seq %d prev %s != head %s", ErrChainBreak, ev.Seq, ev.PrevHash, j.head)}
		}
		if err := ev.VerifyHash(); err != nil {
			return &CorruptionError{Path: s.path, Seq: j.nextSeq, Err: fmt.Errorf("%w: %w", ErrChainBreak, err)}
		}
		j.head = ev.Hash
		j.nextSeq++
		good += int64(len(line)) + 1
		return nil
	})
	return good, err
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

// rangeCompleteLines reads newline-terminated JSONL records without
// bufio.Scanner's token ceiling. It still matches scanCompleteLines' important
// recovery behavior: a trailing unterminated line is discarded because journal
// appends only become committed records once the full line and '\n' are durable.
func rangeCompleteLines(path string, r io.Reader, fn func([]byte) error) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var line []byte
	for {
		frag, err := br.ReadSlice('\n')
		switch {
		case err == nil:
			frag = frag[:len(frag)-1]
			if line != nil {
				line = append(line, frag...)
				if err := fn(line); err != nil {
					return err
				}
				line = nil
				continue
			}
			if err := fn(frag); err != nil {
				return err
			}
		case errors.Is(err, bufio.ErrBufferFull):
			line = append(line, frag...)
		case errors.Is(err, io.EOF):
			return nil
		default:
			return fmt.Errorf("journal: read %s: %w", path, err)
		}
	}
}
