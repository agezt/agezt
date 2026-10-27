// SPDX-License-Identifier: MIT

package atomicfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestWriteFile_CreatesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.json")

	if err := WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatalf("WriteFile(create): %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "v1" {
		t.Fatalf("content = %q, want v1", got)
	}

	if err := WriteFile(path, []byte("v2"), 0o644); err != nil {
		t.Fatalf("WriteFile(replace): %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "v2" {
		t.Fatalf("content = %q, want v2", got)
	}

	// No temp litter left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

func TestWriteFile_Mode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "secret")
	if err := WriteFile(path, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

// TestWriteFile_ReassertsTheModeOfAnExistingFile: the mode is the writer's
// promise on EVERY write, not only at creation.
//
// os.WriteFile applies its perm argument only when it CREATES the file — an
// existing file keeps whatever mode it had. So an export written
// world-readable once stayed world-readable forever, and a re-export could
// not tighten it. The atomic writer chmods the temp before the rename, so
// the target's mode is the one the caller asked for on every write.
//
// The mode bits do not exist on Windows (Go maps only the read-only bit), so
// the assertion is POSIX-only.
func TestWriteFile_ReassertsTheModeOfAnExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "export")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600 — the writer must re-assert the mode on every write", perm)
	}
}

func TestWriteFile_MissingDir(t *testing.T) {
	err := WriteFile(filepath.Join(t.TempDir(), "no", "such", "dir", "f"), []byte("x"), 0o644)
	if err == nil {
		t.Fatal("expected error for missing parent directory")
	}
}

// TestWriteStream_WritesAndReplaces: the streaming writer publishes the
// bytes fn produced, and replaces an existing target wholesale.
//
// The stream is the reason the API exists: a tar+gzip archive is written as
// it is produced, not buffered into a []byte first.
func TestWriteStream_WritesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stream.bin")

	if err := WriteStream(path, 0o644, func(w io.Writer) error {
		_, err := io.WriteString(w, "first")
		return err
	}); err != nil {
		t.Fatalf("WriteStream(create): %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "first" {
		t.Fatalf("content = %q, want first", got)
	}

	if err := WriteStream(path, 0o644, func(w io.Writer) error {
		_, err := io.WriteString(w, "second")
		return err
	}); err != nil {
		t.Fatalf("WriteStream(replace): %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "second" {
		t.Fatalf("content = %q, want second", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

// TestWriteStream_FailureLeavesTheTargetUntouched: a stream that fails does
// not touch the target.
//
// This is the guarantee a plain os.Create cannot give, and it is the reason
// the backup command's "remove the output on error" cleanup had to go: under
// an atomic write the previous good file is still there, and deleting it
// would destroy exactly what the operator needs.
func TestWriteStream_FailureLeavesTheTargetUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.tar.gz")
	if err := os.WriteFile(path, []byte("the previous good archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	sentinel := errors.New("the stream failed")
	err := WriteStream(path, 0o600, func(w io.Writer) error {
		if _, werr := io.WriteString(w, "half an archive"); werr != nil {
			return werr
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want the stream's own error", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "the previous good archive" {
		t.Fatalf("the failed stream changed the target: %q", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "archive.tar.gz" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("the directory holds %v, want exactly [archive.tar.gz]", names)
	}
}

// TestWriteStream_Mode: the mode is the writer's promise, applied to the
// temp before the rename.
//
// The mode bits do not exist on Windows (Go maps only the read-only bit), so
// the assertion is POSIX-only.
func TestWriteStream_Mode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "stream")
	if err := WriteStream(path, 0o600, func(w io.Writer) error {
		_, err := io.WriteString(w, "s")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

func TestWriteStream_MissingDir(t *testing.T) {
	err := WriteStream(filepath.Join(t.TempDir(), "no", "such", "dir", "f"), 0o644, func(w io.Writer) error {
		_, werr := io.WriteString(w, "x")
		return werr
	})
	if err == nil {
		t.Fatal("expected error for missing parent directory")
	}
}

// TestWriteStream_ConcurrentSameTarget: concurrent streams to one target
// never interleave. The final content is one writer's bytes, never a mix.
func TestWriteStream_ConcurrentSameTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contested")
	var wg sync.WaitGroup
	var okCount atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			payload := strings.Repeat(string(rune('a'+n)), 128)
			if err := WriteStream(path, 0o644, func(w io.Writer) error {
				_, werr := io.WriteString(w, payload)
				return werr
			}); err == nil {
				okCount.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if okCount.Load() == 0 {
		t.Fatal("no writer succeeded")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 128 || strings.Count(string(got), string(got[0])) != 128 {
		t.Fatalf("torn write: %q", got[:min(16, len(got))])
	}
}

func TestWriteFile_ConcurrentSameTarget(t *testing.T) { // Unique temp names mean concurrent writers can't rename each other's
	// half-written temp into place: the final content is one writer's value,
	// never interleaved garbage. (On Windows a rename can transiently fail
	// with "Access is denied" while another replace is in flight — losing a
	// race is acceptable, torn content is not.)
	path := filepath.Join(t.TempDir(), "contested")
	var wg sync.WaitGroup
	var okCount atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			payload := strings.Repeat(string(rune('a'+n)), 128)
			if err := WriteFile(path, []byte(payload), 0o644); err == nil {
				okCount.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if okCount.Load() == 0 {
		t.Fatal("no writer succeeded")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 128 || strings.Count(string(got), string(got[0])) != 128 {
		t.Fatalf("torn write: %q", got[:min(16, len(got))])
	}
}
