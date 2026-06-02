// SPDX-License-Identifier: MIT

package plugin_test

// Live integration proof for the M177 stdout frame bound: a real child
// process that floods stdout with an unterminated blob must be torn
// down by the host (Spawn fails) rather than driving the daemon to OOM.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/plugin"
)

var (
	floodBinaryOnce sync.Once
	floodBinaryPath string
	floodBinaryErr  error
)

func buildFloodPlugin(t *testing.T) string {
	t.Helper()
	floodBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "agezt-flood-test-")
		if err != nil {
			floodBinaryErr = err
			return
		}
		binName := "floodplugin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		out := filepath.Join(dir, binName)
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = "testdata/floodplugin"
		if buildOut, err := cmd.CombinedOutput(); err != nil {
			floodBinaryErr = fmt.Errorf("build floodplugin: %v\n%s", err, buildOut)
			return
		}
		floodBinaryPath = out
	})
	if floodBinaryErr != nil {
		t.Fatalf("buildFloodPlugin: %v", floodBinaryErr)
	}
	return floodBinaryPath
}

// TestSpawn_FloodedStdoutTearsDownPlugin — a plugin that writes a 2 MiB
// unterminated frame must be killed by the bounded reader, not OOM the
// daemon. With a small MaxFrameBytes the very first frame (the
// initialize reply the host waits for) overflows, so Spawn returns an
// error rather than hanging or allocating unbounded.
func TestSpawn_FloodedStdoutTearsDownPlugin(t *testing.T) {
	bin := buildFloodPlugin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p, err := plugin.Spawn(ctx, plugin.Config{
		Path:          bin,
		MaxFrameBytes: 256 << 10, // well above bufio's 4 KiB; below the 2 MiB flood
		InitTimeout:   3 * time.Second,
	})
	if err == nil {
		p.Close()
		t.Fatal("Spawn succeeded on a flooding plugin; expected teardown")
	}
	// The failure must be attributable to the frame bound, not merely an
	// init timeout — proves the reader gave up at the cap.
	if !strings.Contains(err.Error(), "frame exceeds max size") {
		t.Errorf("Spawn error = %v; want it to mention the frame size bound", err)
	}
}
