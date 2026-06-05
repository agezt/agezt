// SPDX-License-Identifier: MIT

package sdk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// harness runs ServeRW over a pair of synchronous pipes and lets a test
// act as the host: send request frames in, receive response frames out.
type harness struct {
	t    *testing.T
	inW  *io.PipeWriter
	sc   *bufio.Scanner
	done chan error
}

func newHarness(t *testing.T, tools ...Tool) *harness {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	h := &harness{t: t, inW: inW, sc: bufio.NewScanner(outR), done: make(chan error, 1)}
	go func() {
		err := ServeRW(context.Background(), inR, outW, tools...)
		outW.Close()
		h.done <- err
	}()
	return h
}

func (h *harness) send(f frame) {
	h.t.Helper()
	raw, err := json.Marshal(f)
	if err != nil {
		h.t.Fatalf("marshal frame: %v", err)
	}
	raw = append(raw, '\n')
	if _, err := h.inW.Write(raw); err != nil {
		h.t.Fatalf("send: %v", err)
	}
}

func (h *harness) recv() frame {
	h.t.Helper()
	if !h.sc.Scan() {
		h.t.Fatalf("recv: no frame (%v)", h.sc.Err())
	}
	var f frame
	if err := json.Unmarshal(h.sc.Bytes(), &f); err != nil {
		h.t.Fatalf("recv: bad frame %q: %v", h.sc.Text(), err)
	}
	return f
}

func (h *harness) invoke(name string, input string) frame {
	h.t.Helper()
	params, _ := json.Marshal(invokeParams{Name: name, Input: json.RawMessage(input)})
	h.send(frame{ID: "q1", Method: methodInvoke, Params: params})
	return h.recv()
}

func (h *harness) shutdown() {
	h.t.Helper()
	h.send(frame{ID: "end", Method: methodShutdown})
	h.inW.Close()
	if err := <-h.done; err != nil {
		h.t.Fatalf("serve returned error: %v", err)
	}
}

func decodeResult(t *testing.T, f frame) invokeResult {
	t.Helper()
	if f.Error != "" {
		t.Fatalf("expected a result, got protocol error: %s", f.Error)
	}
	var r invokeResult
	if err := json.Unmarshal(f.Result, &r); err != nil {
		t.Fatalf("decode invoke result: %v", err)
	}
	return r
}

func echoTool() Tool {
	return Tool{
		Name:        "echo",
		Description: "echoes input",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
		Handle: func(ctx context.Context, input json.RawMessage) (Result, error) {
			return Text("echo: " + string(input)), nil
		},
	}
}

func TestInitialize_ListsRegisteredTools(t *testing.T) {
	h := newHarness(t, echoTool(), Tool{
		Name:   "noschema",
		Handle: func(ctx context.Context, in json.RawMessage) (Result, error) { return Text("ok"), nil },
	})
	defer h.shutdown()

	h.send(frame{ID: "i1", Method: methodInitialize})
	f := h.recv()
	if f.ID != "i1" {
		t.Fatalf("id = %q, want i1", f.ID)
	}
	var got initResult
	if err := json.Unmarshal(f.Result, &got); err != nil {
		t.Fatalf("decode init result: %v", err)
	}
	if len(got.Tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(got.Tools))
	}
	byName := map[string]toolDef{}
	for _, d := range got.Tools {
		byName[d.Name] = d
	}
	if byName["echo"].Description != "echoes input" {
		t.Errorf("echo description = %q", byName["echo"].Description)
	}
	// A tool with no schema gets the open-object default.
	if got := string(byName["noschema"].InputSchema); got != `{"type":"object"}` {
		t.Errorf("default schema = %s, want open object", got)
	}
}

func TestInvoke_Success(t *testing.T) {
	h := newHarness(t, echoTool())
	defer h.shutdown()

	r := decodeResult(t, h.invoke("echo", `{"text":"hi"}`))
	if r.IsError {
		t.Fatalf("unexpected IsError; output=%q", r.Output)
	}
	if !strings.Contains(r.Output, `"text":"hi"`) {
		t.Errorf("output = %q, want echoed input", r.Output)
	}
}

func TestInvoke_HandlerErrorMapsToToolError(t *testing.T) {
	h := newHarness(t, Tool{
		Name:   "boom",
		Handle: func(ctx context.Context, in json.RawMessage) (Result, error) { return Result{}, io.ErrUnexpectedEOF },
	})
	defer h.shutdown()

	r := decodeResult(t, h.invoke("boom", `{}`))
	if !r.IsError {
		t.Fatal("expected IsError for handler error")
	}
	if r.Output != io.ErrUnexpectedEOF.Error() {
		t.Errorf("output = %q, want the error text", r.Output)
	}
}

func TestInvoke_ErrorfResult(t *testing.T) {
	h := newHarness(t, Tool{
		Name:   "deny",
		Handle: func(ctx context.Context, in json.RawMessage) (Result, error) { return Errorf("nope: %d", 42), nil },
	})
	defer h.shutdown()

	r := decodeResult(t, h.invoke("deny", `{}`))
	if !r.IsError || r.Output != "nope: 42" {
		t.Fatalf("got %+v, want IsError with formatted output", r)
	}
}

func TestInvoke_UnknownTool(t *testing.T) {
	h := newHarness(t, echoTool())
	defer h.shutdown()

	f := h.invoke("ghost", `{}`)
	if !strings.Contains(f.Error, "unknown tool") {
		t.Fatalf("error = %q, want unknown tool", f.Error)
	}
}

func TestInvoke_BadParams(t *testing.T) {
	h := newHarness(t, echoTool())
	defer h.shutdown()

	// Valid JSON, but the wrong shape: a bare number can't decode into
	// the invoke params object.
	h.send(frame{ID: "q1", Method: methodInvoke, Params: json.RawMessage(`123`)})
	f := h.recv()
	if !strings.Contains(f.Error, "bad params") {
		t.Fatalf("error = %q, want bad params", f.Error)
	}
}

func TestUnknownMethod(t *testing.T) {
	h := newHarness(t, echoTool())
	defer h.shutdown()

	h.send(frame{ID: "x", Method: "frobnicate"})
	f := h.recv()
	if !strings.Contains(f.Error, "unknown method") {
		t.Fatalf("error = %q, want unknown method", f.Error)
	}
}

func TestInvoke_PanicIsContainedAndPluginSurvives(t *testing.T) {
	h := newHarness(t,
		Tool{Name: "panic", Handle: func(ctx context.Context, in json.RawMessage) (Result, error) { panic("kaboom") }},
		echoTool(),
	)
	defer h.shutdown()

	// The panicking tool returns a tool error, not a crash.
	r := decodeResult(t, h.invoke("panic", `{}`))
	if !r.IsError || !strings.Contains(r.Output, "panicked") {
		t.Fatalf("got %+v, want contained panic", r)
	}
	// The plugin is still alive: a subsequent call works.
	r2 := decodeResult(t, h.invoke("echo", `{"ok":true}`))
	if r2.IsError {
		t.Fatalf("plugin did not survive the panic: %+v", r2)
	}
}

func TestInvoke_ProgressThenResult(t *testing.T) {
	h := newHarness(t, Tool{
		Name: "work",
		Handle: func(ctx context.Context, in json.RawMessage) (Result, error) {
			Emit(ctx, "step 1")
			Emit(ctx, "step 2")
			return Text("done"), nil
		},
	})
	defer h.shutdown()

	params, _ := json.Marshal(invokeParams{Name: "work", Input: json.RawMessage(`{}`)})
	h.send(frame{ID: "q1", Method: methodInvoke, Params: params})

	// Two progress frames (no Result/Error), then the terminal result —
	// all share the request id and arrive in order.
	for _, want := range []string{"step 1", "step 2"} {
		f := h.recv()
		if f.ID != "q1" || f.Progress != want || len(f.Result) != 0 {
			t.Fatalf("progress frame = %+v, want progress %q", f, want)
		}
	}
	r := decodeResult(t, h.recv())
	if r.Output != "done" {
		t.Errorf("final output = %q, want done", r.Output)
	}
}

func TestEmit_OutsideHandlerIsNoOp(t *testing.T) {
	// Emit with a bare context must not panic and must do nothing.
	Emit(context.Background(), "ignored")
}

func TestCallHost_RoundTrip(t *testing.T) {
	h := newHarness(t, Tool{
		Name: "viahost",
		Handle: func(ctx context.Context, in json.RawMessage) (Result, error) {
			out, err := CallHost(ctx, "double", in)
			if err != nil {
				return Result{}, err
			}
			return Text("via host: " + out), nil
		},
	})
	defer h.shutdown()

	params, _ := json.Marshal(invokeParams{Name: "viahost", Input: json.RawMessage(`"x"`)})
	h.send(frame{ID: "q1", Method: methodInvoke, Params: params})

	// The plugin asks the host to invoke "double".
	cb := h.recv()
	if cb.Method != methodHostInvoke {
		t.Fatalf("callback method = %q, want host/invoke", cb.Method)
	}
	var cp invokeParams
	if err := json.Unmarshal(cb.Params, &cp); err != nil {
		t.Fatalf("decode callback params: %v", err)
	}
	if cp.Name != "double" {
		t.Fatalf("host tool = %q, want double", cp.Name)
	}
	// The host replies with a result frame (no method) for the callback id.
	hostOut, _ := json.Marshal(invokeResult{Output: "xx"})
	h.send(frame{ID: cb.ID, Result: hostOut})

	// The handler returns, weaving the host's output in.
	r := decodeResult(t, h.recv())
	if r.Output != "via host: xx" {
		t.Errorf("output = %q, want host output woven in", r.Output)
	}
}

func TestCallHost_HostError(t *testing.T) {
	h := newHarness(t, Tool{
		Name: "viahost",
		Handle: func(ctx context.Context, in json.RawMessage) (Result, error) {
			_, err := CallHost(ctx, "forbidden", in)
			if err != nil {
				return Errorf("call failed: %v", err), nil
			}
			return Text("unexpected success"), nil
		},
	})
	defer h.shutdown()

	params, _ := json.Marshal(invokeParams{Name: "viahost", Input: json.RawMessage(`{}`)})
	h.send(frame{ID: "q1", Method: methodInvoke, Params: params})

	cb := h.recv()
	// Host refuses the callback with a protocol error.
	h.send(frame{ID: cb.ID, Error: "tool not allowed"})

	r := decodeResult(t, h.recv())
	if !r.IsError || !strings.Contains(r.Output, "tool not allowed") {
		t.Fatalf("got %+v, want surfaced host error", r)
	}
}

func TestCallHost_OutsideHandler(t *testing.T) {
	if _, err := CallHost(context.Background(), "x", nil); err == nil {
		t.Fatal("expected error calling CallHost outside a handler")
	}
}

func TestReadFrame_CapsOversizedFrame(t *testing.T) {
	const max = 1024
	// A frame larger than max with NO terminating newline: without the
	// cap, readFrame would grow the buffer unbounded. With it, the read
	// errors once the accumulated bytes exceed max.
	huge := strings.Repeat("x", max*4) // no '\n'
	_, err := readFrame(bufio.NewReader(strings.NewReader(huge)), max)
	if err == nil {
		t.Fatal("oversized frame should return an error, not grow unbounded")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should report the cap, got %v", err)
	}
}

func TestReadFrame_NormalFrameUnderCap(t *testing.T) {
	const max = 1024
	want := `{"id":"a","method":"initialize"}`
	got, err := readFrame(bufio.NewReader(strings.NewReader(want+"\n")), max)
	if err != nil {
		t.Fatalf("under-cap frame: %v", err)
	}
	if strings.TrimRight(string(got), "\n") != want {
		t.Errorf("frame=%q want %q", got, want)
	}
}

func TestServeRW_BlankLinesSkipped(t *testing.T) {
	// Stray blank lines must not emit a spurious empty-id error frame;
	// the trailing shutdown ends the loop cleanly with no output.
	var out bytes.Buffer
	in := "\n  \n\n" + `{"id":"s","method":"shutdown"}` + "\n"
	if err := ServeRW(context.Background(), strings.NewReader(in), &out, echoTool()); err != nil {
		t.Fatalf("ServeRW: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("blank lines produced output (expected none): %q", out.String())
	}
}

func TestServeRW_RejectsBadToolRegistration(t *testing.T) {
	bad := []struct {
		name  string
		tools []Tool
	}{
		{"no name", []Tool{{Handle: func(context.Context, json.RawMessage) (Result, error) { return Text(""), nil }}}},
		{"no handler", []Tool{{Name: "x"}}},
		{"duplicate", []Tool{echoTool(), echoTool()}},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			err := ServeRW(context.Background(), strings.NewReader(""), io.Discard, tc.tools...)
			if err == nil {
				t.Fatal("expected registration error")
			}
		})
	}
}

func TestServeRW_EOFReturnsCleanly(t *testing.T) {
	// No shutdown frame — just an immediate EOF on input.
	err := ServeRW(context.Background(), strings.NewReader(""), io.Discard, echoTool())
	if err != nil {
		t.Fatalf("EOF should be a clean return, got %v", err)
	}
}

func TestServeRW_ContextCancelStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A cancelled context returns at the first loop check without
	// consuming input.
	err := ServeRW(ctx, strings.NewReader(`{"id":"a","method":"initialize"}`+"\n"), io.Discard, echoTool())
	if err != nil {
		t.Fatalf("cancelled context should return nil, got %v", err)
	}
}
