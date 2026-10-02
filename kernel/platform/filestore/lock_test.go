// SPDX-License-Identifier: MIT

package filestore

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSaveAndDirsArePrivate pins the privacy posture: files 0600, directories
// 0700, and a pre-existing 0755 directory / 0644 file tightened on next use.
func TestSaveAndDirsArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode bits")
	}
	dir := filepath.Join(t.TempDir(), "legacy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "s.json")
	if err := os.WriteFile(path, []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out []rec
	if _, err := LoadFrom(dir, "s.json", &out); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, []rec{{Name: "a"}}); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{dir: DirPerm, path: FilePerm} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", p, got, want)
		}
	}
}

// TestLockExcludesGoroutines: the critical sections of concurrent holders never
// overlap. A read-modify-write counter would lose increments otherwise.
func TestLockExcludesGoroutines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	var inside, overlaps atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				unlock, err := Lock(path)
				if err != nil {
					t.Error(err)
					return
				}
				if inside.Add(1) != 1 {
					overlaps.Add(1)
				}
				time.Sleep(100 * time.Microsecond)
				inside.Add(-1)
				unlock()
			}
		}()
	}
	wg.Wait()
	if n := overlaps.Load(); n != 0 {
		t.Fatalf("%d overlapping critical sections", n)
	}
}

const helperEnv = "AGEZT_STORE_LOCK_HELPER"

// TestLockExcludesProcesses is the case the lock exists for: the daemon and
// `agt` are different processes. A child process takes the lock and holds it;
// this process must block until the child lets go.
func TestLockExcludesProcesses(t *testing.T) {
	if os.Getenv(helperEnv) != "" {
		t.Skip("helper")
	}
	path := filepath.Join(t.TempDir(), "vault.json")
	const hold = 400 * time.Millisecond
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$")
	cmd.Env = append(os.Environ(), helperEnv+"="+path, "AGEZT_STORE_LOCK_HOLD="+hold.String())
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Wait() }()
	sc := bufio.NewScanner(stdout)
	for sc.Scan() && sc.Text() != "locked" {
	}
	start := time.Now()
	unlock, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	waited := time.Since(start)
	unlock()
	if waited < hold/2 {
		t.Fatalf("acquired after %v while another process held the lock for %v", waited, hold)
	}
}

// TestLockHelperProcess is the child half of TestLockExcludesProcesses.
func TestLockHelperProcess(t *testing.T) {
	path := os.Getenv(helperEnv)
	if path == "" {
		t.Skip("only runs as a helper process")
	}
	hold, _ := time.ParseDuration(os.Getenv("AGEZT_STORE_LOCK_HOLD"))
	unlock, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout.WriteString("locked\n")
	time.Sleep(hold)
	unlock()
}
