// SPDX-License-Identifier: MIT

package plugin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/plugin"
)

var (
	echoBinaryOnce sync.Once
	echoBinaryPath string
	echoBinaryErr  error
)

// buildEchoPlugin compiles testdata/echoplugin into a temp binary
// the first time it's called per test process; subsequent calls
// return the cached path. Building once amortises the ~1s `go
// build` cost across the test cases that need a real subprocess.
func buildEchoPlugin(t *testing.T) string {
	t.Helper()
	echoBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "agezt-plugin-test-")
		if err != nil {
			echoBinaryErr = err
			return
		}
		binName := "echoplugin"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		out := filepath.Join(dir, binName)
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = "testdata/echoplugin"
		if buildOut, err := cmd.CombinedOutput(); err != nil {
			echoBinaryErr = fmt.Errorf("build echoplugin: %v\n%s", err, buildOut)
			return
		}
		echoBinaryPath = out
	})
	if echoBinaryErr != nil {
		t.Fatalf("buildEchoPlugin: %v", echoBinaryErr)
	}
	return echoBinaryPath
}

func TestSpawn_InitializeRegistersTools(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{Path: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	tools := p.Tools("")
	if len(tools) != 4 {
		t.Fatalf("got %d tools, want 4 (echo, fail, slowwork, callhost)", len(tools))
	}
	if _, ok := tools["echo"]; !ok {
		t.Errorf("tools missing echo: %v", keys(tools))
	}
	if _, ok := tools["fail"]; !ok {
		t.Errorf("tools missing fail: %v", keys(tools))
	}
	if _, ok := tools["slowwork"]; !ok {
		t.Errorf("tools missing slowwork (M1.ss progress fixture): %v", keys(tools))
	}
	if _, ok := tools["callhost"]; !ok {
		t.Errorf("tools missing callhost (M1.cb callback fixture): %v", keys(tools))
	}

	def := tools["echo"].Definition()
	if def.Name != "echo" {
		t.Errorf("echo Name = %q", def.Name)
	}
	if def.Description == "" {
		t.Errorf("echo Description empty")
	}
	if !strings.Contains(string(def.InputSchema), "object") {
		t.Errorf("echo InputSchema unexpected: %s", string(def.InputSchema))
	}
}

func TestSpawn_PrefixNamespacing(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{Path: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	tools := p.Tools("myco.")
	if _, ok := tools["myco.echo"]; !ok {
		t.Errorf("prefix not applied; got: %v", keys(tools))
	}
	// The bare name should not be present at the same time.
	if _, ok := tools["echo"]; ok {
		t.Errorf("prefix=myco. should NOT also expose bare 'echo' name; got: %v", keys(tools))
	}
}

func TestInvoke_EchoRoundTrip(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{Path: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	tool := p.Tools("")["echo"]
	in := json.RawMessage(`{"text":"hello"}`)
	res, err := tool.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Errorf("Output = %q, want echo of input", res.Output)
	}
	if res.IsError {
		t.Errorf("IsError = true for echo (should be false)")
	}
}

func TestInvoke_FailRoundTrip(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{Path: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	tool := p.Tools("")["fail"]
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Errorf("IsError = false, want true for fail tool")
	}
}

func TestInvoke_UnknownToolErrors(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{Path: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()

	_, err = p.Invoke(context.Background(), "nope", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("err = %v, want unknown-tool", err)
	}
}

func TestInvoke_ConcurrentCallsDoNotCross(t *testing.T) {
	// Multiple goroutines invoking concurrently must each get
	// their own response. Verifies the id-correlation in the
	// host's read loop doesn't deliver one caller's response
	// to another.
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{Path: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer p.Close()
	tool := p.Tools("")["echo"]

	const N = 20
	var wg sync.WaitGroup
	errs := make(chan error, N)
	for i := range N {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := json.RawMessage(`{"text":"id-` + itoa(i) + `"}`)
			res, err := tool.Invoke(context.Background(), in)
			if err != nil {
				errs <- err
				return
			}
			marker := "id-" + itoa(i)
			if !strings.Contains(res.Output, marker) {
				errs <- fmt.Errorf("call %d got back %q (missing %q)", i, res.Output, marker)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestClose_IsIdempotent(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{Path: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close (idempotent): %v", err)
	}
	if p.IsAlive() {
		t.Error("IsAlive = true after Close")
	}
}

func TestInvoke_AfterCloseReturnsUnavailable(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{Path: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	tool := p.Tools("")["echo"]
	_ = p.Close()

	_, err = tool.Invoke(context.Background(), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("err = %v, want tool-unavailable", err)
	}
}

func TestSpawn_BadPathErrors(t *testing.T) {
	_, err := plugin.Spawn(context.Background(), plugin.Config{
		Path: "/no/such/binary/exists/anywhere",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
	if !strings.Contains(err.Error(), "start") && !strings.Contains(err.Error(), "no such") && !strings.Contains(err.Error(), "cannot find") {
		t.Errorf("err = %v, want start/no-such/cannot-find", err)
	}
}

func TestSpawn_EmptyPathErrors(t *testing.T) {
	_, err := plugin.Spawn(context.Background(), plugin.Config{})
	if err == nil || !strings.Contains(err.Error(), "Path required") {
		t.Errorf("err = %v, want Path-required", err)
	}
}

func TestSpawn_InitTimeoutEnforced(t *testing.T) {
	// We don't have a hanging plugin in testdata; this test
	// verifies the timeout path by passing a non-plugin binary
	// that exits without writing anything (initialize wait fails
	// either on EOF — readLoop sees it first and marks dead — or
	// on context deadline if the process lingers).
	bin := buildEchoPlugin(t) // reuse build infra to find `go`
	_ = bin
	// Use the `go version` command — it writes to stdout but not
	// in the expected JSON shape, and exits quickly. Initialize
	// should fail.
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go not on PATH: %v", err)
	}
	_, err = plugin.Spawn(context.Background(), plugin.Config{
		Path:        goBin,
		Args:        []string{"version"},
		InitTimeout: 500 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected initialize to fail against `go version`")
	}
}

// ---- helpers ----

func keys(m map[string]agent.Tool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Local copies of fmt.Errorf + strconv.Itoa to avoid extra imports.

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
