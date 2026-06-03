// SPDX-License-Identifier: MIT

package sdk_test

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

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/plugin"
)

// This is the live proof for the SDK: an SDK-authored plugin, compiled
// to a real binary, spawned and driven by the actual kernel plugin host
// over OS pipes — initialize, invoke (success + error), progress
// streaming, and a host callback all exercised end-to-end. If the SDK's
// wire behaviour drifted from the host's expectations, this fails.

var (
	greetOnce sync.Once
	greetPath string
	greetErr  error
)

func buildGreetPlugin(t *testing.T) string {
	t.Helper()
	greetOnce.Do(func() {
		dir, err := os.MkdirTemp("", "agezt-sdk-example-")
		if err != nil {
			greetErr = err
			return
		}
		bin := "greet"
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		out := filepath.Join(dir, bin)
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = filepath.Join("example", "greet")
		if buildOut, err := cmd.CombinedOutput(); err != nil {
			greetErr = fmt.Errorf("build greet: %v\n%s", err, buildOut)
			return
		}
		greetPath = out
	})
	if greetErr != nil {
		t.Fatalf("buildGreetPlugin: %v", greetErr)
	}
	return greetPath
}

// upperTool is a trivial in-host tool the example plugin calls back
// into via sdk.CallHost / host/invoke.
type upperTool struct{}

func (upperTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "upper", Description: "uppercases text"}
}

func (upperTool) Invoke(_ context.Context, input json.RawMessage) (agent.Result, error) {
	var in struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(input, &in)
	return agent.Result{Output: strings.ToUpper(in.Text)}, nil
}

func spawnGreet(t *testing.T) *plugin.Plugin {
	t.Helper()
	bin := buildGreetPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{
		Path:      bin,
		HostTools: map[string]agent.Tool{"upper": upperTool{}},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	return p
}

func TestSDKExample_InitializeRegistersTools(t *testing.T) {
	p := spawnGreet(t)
	defer p.Close()

	tools := p.Tools("")
	for _, name := range []string{"greet", "slow", "shout"} {
		if _, ok := tools[name]; !ok {
			t.Errorf("missing tool %q (have %d tools)", name, len(tools))
		}
	}
}

func TestSDKExample_InvokeSuccessAndError(t *testing.T) {
	p := spawnGreet(t)
	defer p.Close()

	ok, err := p.Invoke(context.Background(), "greet", json.RawMessage(`{"name":"Ada"}`))
	if err != nil {
		t.Fatalf("invoke greet: %v", err)
	}
	if ok.IsError || ok.Output != "Hello, Ada!" {
		t.Fatalf("greet result = %+v, want Hello, Ada!", ok)
	}

	// Missing required field → tool-level error surfaced as IsError,
	// not a transport failure.
	bad, err := p.Invoke(context.Background(), "greet", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("invoke greet(empty): transport error %v", err)
	}
	if !bad.IsError || !strings.Contains(bad.Output, "required") {
		t.Fatalf("greet(empty) = %+v, want IsError about required name", bad)
	}
}

func TestSDKExample_ProgressStreaming(t *testing.T) {
	p := spawnGreet(t)
	defer p.Close()

	var mu sync.Mutex
	var got []string
	res, err := p.InvokeWithProgress(context.Background(), "slow", json.RawMessage(`{}`), func(line string) {
		mu.Lock()
		got = append(got, line)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("invoke slow: %v", err)
	}
	if res.Output != "done" {
		t.Errorf("slow output = %q, want done", res.Output)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"counting: one", "counting: two", "counting: three"}
	if len(got) != len(want) {
		t.Fatalf("progress lines = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("progress[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSDKExample_HostCallback(t *testing.T) {
	p := spawnGreet(t)
	defer p.Close()

	res, err := p.Invoke(context.Background(), "shout", json.RawMessage(`{"text":"hi there"}`))
	if err != nil {
		t.Fatalf("invoke shout: %v", err)
	}
	if res.IsError || res.Output != "HI THERE" {
		t.Fatalf("shout result = %+v, want HI THERE (via host upper)", res)
	}
}
