// SPDX-License-Identifier: MIT

// file_helpers.go: doRead + doReadRange split off from file.go during the Day 211
// god-file refactor (#137). Public API unchanged.
package file

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)


// ----- ops -----

func (t *Tool) doRead(in fileInput) (agent.Result, error) {
	p, err := t.resolve(in.Path)
	if err != nil {
		return errResult(err.Error()), nil
	}
	info, err := os.Stat(p)
	if err != nil {
		return errResult("stat: " + err.Error()), nil
	}
	if info.IsDir() {
		return errResult(in.Path + " is a directory; use op=list"), nil
	}
	// Line-range read (M117): page a region of a file rather than the whole
	// thing — essential for large files (where the default read truncates to the
	// first MaxReadBytes) and for reading around a `search` hit.
	if in.StartLine > 0 || in.EndLine > 0 {
		return t.doReadRange(in, p)
	}
	if info.Size() > MaxReadBytes {
		// Partial read with a notice.
		f, err := openFileNoFollow(p, os.O_RDONLY, 0, t.root)
		if err != nil {
			return errResult("read: " + err.Error()), nil
		}
		defer f.Close()
		buf, rerr := readUpTo(f, MaxReadBytes)
		if rerr != nil {
			return errResult("read: " + rerr.Error()), nil
		}
		out := fmt.Sprintf("[file truncated: showing first %d of %d bytes]\n%s",
			len(buf), info.Size(), string(buf))
		return fileObservation(in.Path, out), nil
	}
	f, err := openFileNoFollow(p, os.O_RDONLY, 0, t.root)
	if err != nil {
		return errResult("read: " + err.Error()), nil
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return errResult("read: " + err.Error()), nil
	}
	return fileObservation(in.Path, string(data)), nil
}

// defaultReadRangeLines is the window size when only start_line is given.
const defaultReadRangeLines = 200

// maxReadRangeLines caps a single line-range read.
const maxReadRangeLines = 5000

// doReadRange returns lines [start_line, end_line] of a file (M117), bounded by
// maxReadRangeLines and MaxReadBytes. The output is the raw line content (usable
// directly for a follow-up `replace`) under a "[lines X-Y]" header.
func (t *Tool) doReadRange(in fileInput, p string) (agent.Result, error) {
	start := in.StartLine
	if start < 1 {
		start = 1
	}
	end := in.EndLine
	if end <= 0 {
		end = start + defaultReadRangeLines - 1
	}
	if end < start {
		return errResult("read: end_line is before start_line"), nil
	}
	if end-start+1 > maxReadRangeLines {
		end = start + maxReadRangeLines - 1
	}

	f, err := openFileNoFollow(p, os.O_RDONLY, 0, t.root)
	if err != nil {
		return errResult("read: " + err.Error()), nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // tolerate long lines

	var b strings.Builder
	line, written, emitted := 0, 0, 0
	truncated := false
	for sc.Scan() {
		line++
		if line < start {
			continue
		}
		if line > end {
			break
		}
		text := sc.Text()
		if written+len(text)+1 > MaxReadBytes {
			truncated = true
			break
		}
		b.WriteString(text)
		b.WriteByte('\n')
		written += len(text) + 1
		emitted++
	}
	if err := sc.Err(); err != nil {
		return errResult("scan: " + err.Error()), nil
	}
	if emitted == 0 {
		return errResult(fmt.Sprintf("read: no lines in range [%d,%d]; file has %d line(s)", start, end, line)), nil
	}
	header := fmt.Sprintf("[lines %d-%d]", start, start+emitted-1)
	if truncated {
		header += " [truncated at byte cap]"
	}
	return fileObservation(in.Path, header+"\n"+b.String()), nil
}
