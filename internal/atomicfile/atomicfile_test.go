// SPDX-License-Identifier: MIT

package atomicfile

import (
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

func TestWriteFile_MissingDir(t *testing.T) {
	err := WriteFile(filepath.Join(t.TempDir(), "no", "such", "dir", "f"), []byte("x"), 0o644)
	if err == nil {
		t.Fatal("expected error for missing parent directory")
	}
}

func TestWriteFile_ConcurrentSameTarget(t *testing.T) {
	// Unique temp names mean concurrent writers can't rename each other's
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
