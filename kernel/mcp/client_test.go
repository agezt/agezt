// SPDX-License-Identifier: MIT

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeServer speaks just enough MCP over a pipe pair to drive the client:
// initialize → tools/list (one "echo" tool) → tools/call echoes arguments.
// It also emits a stray notification before every response, proving the
// client skips frames that aren't its answer.
func fakeServer(t *testing.T, r io.Reader, w io.Writer) {
	t.Helper()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), maxFrameBytes)
	send := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = w.Write(append(b, '\n'))
	}
	defer func() { _ = sc.Err() }() // EOF ends the fake server; nothing actionable
	for sc.Scan() {
		var req struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil || req.ID == nil {
			continue // notification (e.g. notifications/initialized) — no reply
		}
		send(map[string]any{"jsonrpc": "2.0", "method": "notifications/message", "params": map[string]any{"level": "info"}})
		switch req.Method {
		case "initialize":
			send(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{
				"protocolVersion": protocolVersion, "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "fake"},
			}})
		case "tools/list":
			send(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{
				"tools": []map[string]any{{
					"name": "echo", "description": "echoes text",
					"inputSchema": map[string]any{"type": "object"},
				}},
			}})
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.Name == "boom" {
				send(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -1, "message": "kaput"}})
				continue
			}
			send(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{
				"content": []map[string]any{{"type": "text", "text": "echo: " + string(p.Arguments)}},
				"isError": false,
			}})
		default:
			send(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "no such method"}})
		}
	}
}

func pipeConn(t *testing.T) Conn {
	t.Helper()
	clientOut, serverIn := io.Pipe() // client writes → server reads
	serverOut, clientIn := io.Pipe() // server writes → client reads
	go func() {
		fakeServer(t, clientOut, clientIn)
		_ = clientIn.Close() // EOF on our stdin → hang up our stdout, like a real child exiting
	}()
	c := newClientConn(serverIn, serverOut, func() { _ = clientIn.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.handshake(ctx); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// TestHandshakeAndCall_SkipsNotifications: the full client dialogue over
// in-memory pipes — initialize, tool discovery, a call whose answer arrives
// AFTER an unrelated notification, and a server-side error.
func TestHandshakeAndCall_SkipsNotifications(t *testing.T) {
	c := pipeConn(t)
	tools := c.Tools()
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v, want [echo]", tools)
	}
	out, isErr, err := c.Call(context.Background(), "echo", json.RawMessage(`{"text":"merhaba"}`))
	if err != nil || isErr {
		t.Fatalf("Call: %v / isErr=%v", err, isErr)
	}
	if !strings.Contains(out, `"merhaba"`) {
		t.Fatalf("arguments did not round-trip: %q", out)
	}
	if _, _, err := c.Call(context.Background(), "boom", nil); err == nil {
		t.Fatal("server error not surfaced")
	}
}

// TestCall_ConnectionLost: when the server side dies mid-call, the client
// reports a lost connection instead of hanging.
func TestCall_ConnectionLost(t *testing.T) {
	clientOut, serverIn := io.Pipe()
	serverOut, clientIn := io.Pipe()
	c := newClientConn(serverIn, serverOut, nil)
	t.Cleanup(func() { _ = c.Close() })
	go func() {
		// Swallow the request, then hang up without answering.
		buf := make([]byte, 4096)
		_, _ = clientOut.Read(buf)
		_ = clientIn.Close()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.roundTrip(ctx, "tools/list", map[string]any{}, nil); err == nil {
		t.Fatal("lost connection not reported")
	}
}

// TestScrubbedEnv: the load-bearing safety property — secrets and the whole
// AGEZT_* namespace never reach a spawned server; PATH does.
func TestScrubbedEnv(t *testing.T) {
	t.Setenv("AGEZT_SECRET_PROBE", "leakme")
	t.Setenv("MY_API_KEY", "leakme")
	t.Setenv("PATH", os.Getenv("PATH")) // ensure present
	for _, kv := range scrubbedEnv() {
		up := strings.ToUpper(kv)
		if strings.HasPrefix(up, "AGEZT_") || strings.HasPrefix(up, "MY_API_KEY") {
			t.Fatalf("secret leaked into child env: %s", kv)
		}
	}
	joined := strings.Join(scrubbedEnv(), "\n")
	if !strings.Contains(strings.ToUpper(joined), "PATH=") {
		t.Fatal("PATH missing from child env")
	}
}

// pythonPath finds a Python interpreter for the live subprocess test.
func pythonPath() string {
	for _, name := range []string{"python", "python3"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// fakeServerPy is a complete stdio MCP server in ~25 lines of Python — the
// live-subprocess counterpart of fakeServer, also used by the smoke.
const fakeServerPy = `import sys, json
def send(o):
    sys.stdout.write(json.dumps(o) + "\n"); sys.stdout.flush()
for line in sys.stdin:
    line = line.strip()
    if not line: continue
    m = json.loads(line)
    mid, meth = m.get("id"), m.get("method")
    if mid is None: continue
    if meth == "initialize":
        send({"jsonrpc":"2.0","id":mid,"result":{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"fake","version":"1"}}})
    elif meth == "tools/list":
        send({"jsonrpc":"2.0","id":mid,"result":{"tools":[{"name":"greet","description":"greets a name","inputSchema":{"type":"object","properties":{"name":{"type":"string"}}}}]}})
    elif meth == "tools/call":
        args = m["params"].get("arguments") or {}
        send({"jsonrpc":"2.0","id":mid,"result":{"content":[{"type":"text","text":"hello " + str(args.get("name","?"))}],"isError":False}})
    else:
        send({"jsonrpc":"2.0","id":mid,"error":{"code":-32601,"message":"nope"}})
`

// TestDial_LivePythonServer drives the REAL path — spawn, handshake, tool
// discovery, a call, close — against a python child process. Skipped when no
// Python is installed.
func TestDial_LivePythonServer(t *testing.T) {
	py := pythonPath()
	if py == "" {
		t.Skip("python not installed")
	}
	script := filepath.Join(t.TempDir(), "server.py")
	if err := os.WriteFile(script, []byte(fakeServerPy), 0o600); err != nil {
		t.Fatalf("write server: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := Dial(ctx, py, []string{script})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	tools := conn.Tools()
	if len(tools) != 1 || tools[0].Name != "greet" {
		t.Fatalf("tools = %+v", tools)
	}
	out, isErr, err := conn.Call(ctx, "greet", json.RawMessage(`{"name":"ersin"}`))
	if err != nil || isErr || out != "hello ersin" {
		t.Fatalf("Call = %q / %v / %v", out, isErr, err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
