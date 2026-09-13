// SPDX-License-Identifier: MIT

package file

// File-tool WRITE operations + IO helpers: doWrite + doReplace +
// readUpTo + writeAll + atomicWriteFile. Carved out of file.go during
// the Day 180 god-file split so the main file can stay focused on
// constants + Tool struct + lifecycle + Invoke + read operations.
// Public API unchanged.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)
func (t *Tool) doWrite(ctx context.Context, in fileInput, appendMode bool) (agent.Result, error) {
	p, err := t.resolve(in.Path)
	if err != nil {
		return errResult(err.Error()), nil
	}
	action := "file.write"
	if appendMode {
		action = "file.append"
	}
	if err := t.checkpointFileSnapshot(ctx, action, in.Path, p); err != nil {
		return errResult("checkpoint: " + err.Error()), nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return errResult("mkdir parent: " + err.Error()), nil
	}
	// openFileNoFollow closes the narrow TOCTOU between resolve() and the
	// open: a concurrent process could plant a symlink/reparse-point at p
	// after the in-root check, and a plain O_CREATE would follow it out of
	// the workspace (M427 follow-up). On unix this is a kernel-enforced flag
	// (O_NOFOLLOW); on Windows the opened handle's final path is resolved
	// via GetFinalPathNameByHandle and verified against t.root.
	flag := os.O_WRONLY | os.O_CREATE
	if appendMode {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := openFileNoFollow(p, flag, 0o644, t.root)
	if err != nil {
		return errResult("open: " + err.Error()), nil
	}
	defer f.Close()
	if _, err := f.WriteString(in.Content); err != nil {
		return errResult("write: " + err.Error()), nil
	}
	if err := f.Sync(); err != nil {
		return errResult("fsync: " + err.Error()), nil
	}
	verb := "wrote"
	if appendMode {
		verb = "appended"
	}
	return agent.Result{
		Output: fmt.Sprintf("%s %d bytes to %s", verb, len(in.Content), in.Path),
	}, nil
}

// doReplace performs an in-place find/replace edit (M114) — the partial-edit op
// the file tool long deferred. By default `find` must match EXACTLY ONCE so an
// ambiguous edit fails loudly instead of changing the wrong place; set all=true
// to replace every occurrence. This lets an agent edit a file surgically rather
// than read-and-rewrite the whole thing, cutting context cost and clobber risk.
func (t *Tool) doReplace(ctx context.Context, in fileInput) (agent.Result, error) {
	p, err := t.resolve(in.Path)
	if err != nil {
		return errResult(err.Error()), nil
	}
	if in.Find == "" {
		return errResult("replace: 'find' must be non-empty"), nil
	}
	if in.Find == in.Replacement {
		return errResult("replace: 'find' and 'replacement' are identical (no change)"), nil
	}
	info, err := os.Stat(p)
	if err != nil {
		return errResult("stat: " + err.Error()), nil
	}
	if info.IsDir() {
		return errResult("replace: " + in.Path + " is a directory"), nil
	}
	if info.Size() > MaxScanBytes {
		return errResult(fmt.Sprintf("replace: file too large (%d bytes, max %d)", info.Size(), MaxScanBytes)), nil
	}
	data, err := func() ([]byte, error) {
		f, oerr := openFileNoFollow(p, os.O_RDONLY, 0, t.root)
		if oerr != nil {
			return nil, oerr
		}
		defer f.Close()
		return io.ReadAll(f)
	}()
	if err != nil {
		return errResult("read: " + err.Error()), nil
	}
	content := string(data)
	n := strings.Count(content, in.Find)
	if n == 0 {
		return errResult(fmt.Sprintf("replace: 'find' string not found in %s", in.Path)), nil
	}
	if n > 1 && !in.All {
		return errResult(fmt.Sprintf("replace: 'find' matches %d times in %s — make it unique (add surrounding context) or set all=true", n, in.Path)), nil
	}
	count := 1
	updated := strings.Replace(content, in.Find, in.Replacement, 1)
	if in.All {
		count = n
		updated = strings.ReplaceAll(content, in.Find, in.Replacement)
	}
	// Preserve the M440 guard — refuse to edit through a symlink swapped in at p
	// after resolve() — then write ATOMICALLY (temp + rename) so a mid-write failure
	// (ENOSPC, crash) can't truncate or destroy the original. `replace` is the
	// low-clobber surgical-edit op; the prior O_TRUNC zeroed the file before the new
	// bytes landed, so a partial write lost the user's data. rename never follows a
	// symlink at p (it replaces the entry), so the write still cannot escape root.
	if li, lerr := os.Lstat(p); lerr != nil {
		return errResult("write: " + lerr.Error()), nil
	} else if li.Mode()&os.ModeSymlink != 0 {
		return errResult("write: refusing to follow symlink at " + in.Path), nil
	}
	if err := t.checkpointFileSnapshot(ctx, "file.replace", in.Path, p); err != nil {
		return errResult("checkpoint: " + err.Error()), nil
	}
	if err := atomicWriteFile(p, []byte(updated), info.Mode().Perm()); err != nil {
		return errResult("write: " + err.Error()), nil
	}
	delta := len(updated) - len(content)
	return agent.Result{
		Output: fmt.Sprintf("replaced %d occurrence(s) in %s (%+d bytes)", count, in.Path, delta),
	}, nil
}

// readUpTo reads up to max bytes from r, looping past short reads until the buffer
// is full or the stream ends. A single (*os.File).Read may legitimately return
// fewer bytes than requested, so a lone Read could return an unpredictably short
// prefix while the caller claims to show "the first N bytes". EOF/UnexpectedEOF are
// normal end-of-stream (a file shorter than max); any other error is surfaced
// rather than silently presented as truncated content.
func readUpTo(r io.Reader, max int) ([]byte, error) {
	buf := make([]byte, max)
	n, err := io.ReadFull(r, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		err = nil
	}
	return buf[:n], err
}

// writeAll is indirected so tests can simulate a mid-write failure and confirm
// atomicWriteFile leaves the original file intact.
var writeAll = func(f *os.File, b []byte) (int, error) { return f.Write(b) }

// atomicWriteFile writes data to path atomically: a fresh temp file in the same
// directory is written, fsynced, and renamed over path. A mid-write failure
// (ENOSPC, crash) therefore cannot truncate or destroy the existing file — the
// original survives until the rename swaps in the complete new content. The temp
// is created with O_EXCL (never a pre-existing symlink); rename does not follow a
// symlink at path (it replaces the entry), so the write cannot escape the
// directory. perm is applied to the result.
// Kept local (not internal/atomicfile) so the writeAll seam above can inject
// mid-write failures in tests.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".agezt-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename has moved it
	if _, err := writeAll(tmp, data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

