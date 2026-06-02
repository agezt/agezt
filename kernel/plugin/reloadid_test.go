// SPDX-License-Identifier: MIT

package plugin_test

// Live integration proof for M180: the host's correlation-id counter
// must stay monotonic across a Reload, so a post-reload request can
// never reuse an id a pre-reload request already used (response
// confusion). The idechoplugin fixture returns the host-assigned
// request id as its Output, letting us observe the sequence.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/plugin"
)

var (
	idechoBinaryOnce sync.Once
	idechoBinaryPath string
	idechoBinaryErr  error
)

func buildIDEchoPlugin(t *testing.T) string {
	t.Helper()
	idechoBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "agezt-idecho-test-")
		if err != nil {
			idechoBinaryErr = err
			return
		}
		binName := "idechoplugin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		out := filepath.Join(dir, binName)
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = "testdata/idechoplugin"
		if buildOut, err := cmd.CombinedOutput(); err != nil {
			idechoBinaryErr = fmt.Errorf("build idechoplugin: %v\n%s", err, buildOut)
			return
		}
		idechoBinaryPath = out
	})
	if idechoBinaryErr != nil {
		t.Fatalf("buildIDEchoPlugin: %v", idechoBinaryErr)
	}
	return idechoBinaryPath
}

// idNum parses the numeric suffix of a "q-N" correlation id.
func idNum(t *testing.T, out string) int {
	t.Helper()
	var r struct {
		Output string `json:"output"`
	}
	// The tool result Output is the raw id string "q-N".
	id := out
	if strings.HasPrefix(out, "{") {
		if err := json.Unmarshal([]byte(out), &r); err == nil && r.Output != "" {
			id = r.Output
		}
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, "q-"))
	if err != nil {
		t.Fatalf("unparseable id %q: %v", out, err)
	}
	return n
}

func TestReload_CorrelationIDsStayMonotonic(t *testing.T) {
	bin := buildIDEchoPlugin(t)
	ctx := context.Background()
	p, err := plugin.Spawn(ctx, plugin.Config{Path: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	tool := p.Tools("")["idecho"]
	if tool == nil {
		t.Fatal("idecho tool not registered")
	}

	invokeID := func() int {
		res, err := p.Invoke(ctx, "idecho", json.RawMessage(`{}`))
		if err != nil {
			t.Fatalf("invoke: %v", err)
		}
		return idNum(t, res.Output)
	}

	before1 := invokeID()
	before2 := invokeID()
	if before2 <= before1 {
		t.Fatalf("ids not climbing pre-reload: %d then %d", before1, before2)
	}

	if err := p.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	after := invokeID()
	// The crux: a post-reload id must exceed every pre-reload id. With
	// the old nextID.Store(0) reset, `after` would collide with an early
	// pre-reload value (e.g. q-2) instead of climbing past it.
	if after <= before2 {
		t.Errorf("correlation id reset across reload: pre-reload max %d, post-reload %d", before2, after)
	}
}
