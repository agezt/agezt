// SPDX-License-Identifier: MIT

// Package sdk is the official Go authoring kit for agezt tool plugins.
//
// Writing a plugin by hand means implementing the line-delimited JSON
// protocol yourself: the stdin read loop, the request/response frame
// demux, write serialisation across goroutines, progress streaming, and
// host-callback routing. The reference plugin (kernel/plugin/testdata/
// echoplugin) is ~260 lines of exactly that boilerplate. This package
// collapses it so a plugin author writes only their tool logic:
//
//	package main
//
//	import (
//		"context"
//		"encoding/json"
//
//		"github.com/agezt/agezt/plugins/sdk"
//	)
//
//	func main() {
//		sdk.Serve(sdk.Tool{
//			Name:        "greet",
//			Description: "Returns a greeting.",
//			InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`),
//			Handle: func(ctx context.Context, input json.RawMessage) (sdk.Result, error) {
//				var in struct{ Name string `json:"name"` }
//				json.Unmarshal(input, &in)
//				return sdk.Text("hello, " + in.Name), nil
//			},
//		})
//	}
//
// That binary is a complete, spec-conformant agezt plugin. Point a
// daemon at it with AGEZT_PLUGINS and the "greet" tool is live.
//
// # Design constraints (mirrors the host)
//
// This package imports ONLY the Go standard library. It deliberately
// does NOT import kernel/plugin or kernel/agent: a plugin must not have
// to compile against the kernel (DECISIONS B0). The wire types here are
// independent copies of the small plugin-side contract documented in
// kernel/plugin/protocol.go — the same on-disk JSON shape an author in
// Python or Rust would target by hand.
//
// # What Serve handles for you
//
//   - initialize: replies with the tool definitions you registered.
//   - tool/invoke: routes to the matching Handle, on its own goroutine
//     so the read loop stays responsive to concurrent invokes and to
//     host-callback replies.
//   - shutdown: returns cleanly.
//   - Panics inside a handler are recovered and surfaced as a tool error
//     (IsError) rather than crashing the plugin — one bad tool call does
//     not take the process down.
//   - Progress streaming via Emit and host callbacks via CallHost, both
//     keyed to the in-flight request so you never touch a frame id.
package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
)

// Result is what a tool Handle returns to the agent. It mirrors the
// host's InvokeResult.
type Result struct {
	// Output is the tool's textual result, fed back into the agent
	// loop as the tool call's observation.
	Output string
	// IsError marks the call as a tool-level failure. The agent still
	// sees Output (typically an error message) and can react to it.
	// This is distinct from a protocol error — a handler returning a
	// non-nil error is mapped to IsError automatically.
	IsError bool
}

// Text is shorthand for a successful Result carrying s.
func Text(s string) Result { return Result{Output: s} }

// Errorf is shorthand for a tool-level failure Result whose Output is
// the formatted message. Prefer returning a plain error from a Handle
// when you can; Errorf is for when you want IsError without unwinding.
func Errorf(format string, a ...any) Result {
	return Result{Output: fmt.Sprintf(format, a...), IsError: true}
}

// Handler is a plugin author's tool logic. input is the raw JSON the
// agent passed for this call (matching the tool's InputSchema).
//
// Return (Result, nil) for success or a deliberate IsError result.
// Returning a non-nil error is a convenience: it is converted to a
// tool-level failure Result (Output = err.Error(), IsError = true), so
// you rarely need to construct an error Result by hand.
type Handler func(ctx context.Context, input json.RawMessage) (Result, error)

// Tool bundles a tool definition with its handler.
type Tool struct {
	// Name is the tool identifier the agent calls. Required, unique
	// within a plugin.
	Name string
	// Description is shown to the model when it decides which tool to
	// call. A precise one-liner.
	Description string
	// InputSchema is the JSON Schema for the tool's input. Optional;
	// an empty value advertises an open object.
	InputSchema json.RawMessage
	// Handle runs the tool. Required.
	Handle Handler
}

// frame is the union wire shape for both directions, matching the
// plugin-side contract. A frame with Method set is a request from the
// host; a frame without Method is a response to a host/invoke callback
// this plugin originated.
type frame struct {
	ID       string          `json:"id"`
	Method   string          `json:"method,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
	Progress string          `json:"progress,omitempty"`
}

type toolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type initResult struct {
	Tools []toolDef `json:"tools"`
}

type invokeParams struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type invokeResult struct {
	Output  string `json:"output"`
	IsError bool   `json:"is_error,omitempty"`
}

// Wire method names — single source of truth, identical to the host's.
const (
	methodInitialize = "initialize"
	methodInvoke     = "tool/invoke"
	methodShutdown   = "shutdown"
	methodHostInvoke = "host/invoke"
)

// session holds the per-process serving state: the registered tools,
// the serialised writer, and the table of in-flight host callbacks.
type session struct {
	tools map[string]Tool

	wmu sync.Mutex // serialises every write to w (responses, progress, callbacks)
	w   *bufio.Writer

	pendMu  sync.Mutex
	pending map[string]chan callResp
	cbSeq   atomic.Int64
}

type callResp struct {
	result json.RawMessage
	err    string
}

// ctxKey carries the per-invocation state so Emit and CallHost can find
// the session and the request id without the author threading them.
type ctxKey struct{}

type invocation struct {
	s  *session
	id string
}

func fromContext(ctx context.Context) (*invocation, bool) {
	inv, ok := ctx.Value(ctxKey{}).(*invocation)
	return inv, ok && inv != nil
}

// Serve runs the plugin protocol on stdin/stdout until the host sends
// shutdown or the input stream closes. It is the normal entry point —
// call it from main with your tools. The returned error is non-nil only
// on an unexpected I/O failure; a clean shutdown or EOF returns nil.
func Serve(tools ...Tool) error {
	return ServeRW(context.Background(), os.Stdin, os.Stdout, tools...)
}

// ServeRW is Serve against explicit streams. It exists for tests and
// for embedding the plugin loop over a transport other than the process
// stdio (a socket pair, an in-memory pipe). ctx cancellation stops the
// read loop at the next frame boundary.
func ServeRW(ctx context.Context, r io.Reader, w io.Writer, tools ...Tool) error {
	s := &session{
		tools:   make(map[string]Tool, len(tools)),
		w:       bufio.NewWriter(w),
		pending: make(map[string]chan callResp),
	}
	for _, t := range tools {
		if t.Name == "" || t.Handle == nil {
			return fmt.Errorf("sdk: tool %q must have a name and a handler", t.Name)
		}
		if _, dup := s.tools[t.Name]; dup {
			return fmt.Errorf("sdk: tool %q registered more than once", t.Name)
		}
		s.tools[t.Name] = t
	}
	defer s.w.Flush()

	dec := bufio.NewReader(r)
	var wg sync.WaitGroup
	defer wg.Wait() // let in-flight handlers finish writing before returning

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		line, err := dec.ReadBytes('\n')
		if err != nil {
			// EOF or read error: the host went away. Clean exit.
			return nil
		}
		var f frame
		if err := json.Unmarshal(line, &f); err != nil {
			s.writeFrame(frame{ID: f.ID, Error: "bad request: " + err.Error()})
			continue
		}

		// A frame with no Method is a reply to a host/invoke callback
		// we originated — route it to the waiting goroutine.
		if f.Method == "" {
			s.routeCallback(f)
			continue
		}

		switch f.Method {
		case methodInitialize:
			s.writeFrame(frame{ID: f.ID, Result: s.initPayload()})
		case methodInvoke:
			s.dispatchInvoke(ctx, &wg, f)
		case methodShutdown:
			return nil
		default:
			s.writeFrame(frame{ID: f.ID, Error: "unknown method: " + f.Method})
		}
	}
}

func (s *session) initPayload() json.RawMessage {
	defs := make([]toolDef, 0, len(s.tools))
	for _, t := range s.tools {
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		defs = append(defs, toolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}
	raw, _ := json.Marshal(initResult{Tools: defs})
	return raw
}

// dispatchInvoke parses the invoke params and runs the handler on its
// own goroutine so the read loop stays free for concurrent invokes and
// for callback replies. Protocol-level problems (bad params, unknown
// tool) are answered inline as Error frames.
func (s *session) dispatchInvoke(ctx context.Context, wg *sync.WaitGroup, f frame) {
	var p invokeParams
	if err := json.Unmarshal(f.Params, &p); err != nil {
		s.writeFrame(frame{ID: f.ID, Error: "bad params: " + err.Error()})
		return
	}
	t, ok := s.tools[p.Name]
	if !ok {
		s.writeFrame(frame{ID: f.ID, Error: "unknown tool: " + p.Name})
		return
	}

	wg.Go(func() {
		res := s.runHandler(ctx, t, f.ID, p.Input)
		raw, _ := json.Marshal(res)
		s.writeFrame(frame{ID: f.ID, Result: raw})
	})
}

// runHandler executes a tool handler with the invocation bound to ctx,
// recovering panics into a tool-level error so one bad call can't crash
// the plugin (mirrors the kernel's agent panic firewall, M168).
func (s *session) runHandler(ctx context.Context, t Tool, id string, input json.RawMessage) (res invokeResult) {
	defer func() {
		if r := recover(); r != nil {
			res = invokeResult{Output: fmt.Sprintf("tool %q panicked: %v", t.Name, r), IsError: true}
		}
	}()
	callCtx := context.WithValue(ctx, ctxKey{}, &invocation{s: s, id: id})
	out, err := t.Handle(callCtx, input)
	if err != nil {
		return invokeResult{Output: err.Error(), IsError: true}
	}
	return invokeResult{Output: out.Output, IsError: out.IsError}
}

// Emit streams a human-readable progress line for the in-flight tool
// call. It is a no-op when called outside a handler (or after the call
// has returned). Progress frames are advisory; a host that does not
// consume them drops them silently.
func Emit(ctx context.Context, message string) {
	inv, ok := fromContext(ctx)
	if !ok || inv.id == "" {
		return
	}
	inv.s.writeFrame(frame{ID: inv.id, Progress: message})
}

// CallHost invokes a host tool from inside a handler (the host/invoke
// callback direction) and returns its Output. The set of callable host
// tools is configured operator-side; an attempt to call a tool that is
// not allow-listed comes back as an error. CallHost blocks until the
// host replies or ctx is cancelled.
func CallHost(ctx context.Context, toolName string, input json.RawMessage) (string, error) {
	inv, ok := fromContext(ctx)
	if !ok {
		return "", fmt.Errorf("sdk: CallHost called outside a tool handler")
	}
	s := inv.s

	id := "cb-" + strconv.FormatInt(s.cbSeq.Add(1), 10)
	ch := make(chan callResp, 1)
	s.pendMu.Lock()
	s.pending[id] = ch
	s.pendMu.Unlock()
	defer func() {
		s.pendMu.Lock()
		delete(s.pending, id)
		s.pendMu.Unlock()
	}()

	params, _ := json.Marshal(invokeParams{Name: toolName, Input: input})
	s.writeFrame(frame{ID: id, Method: methodHostInvoke, Params: params})

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case resp := <-ch:
		if resp.err != "" {
			return "", fmt.Errorf("host returned error: %s", resp.err)
		}
		var r invokeResult
		if err := json.Unmarshal(resp.result, &r); err != nil {
			return "", fmt.Errorf("sdk: parse host result: %w", err)
		}
		if r.IsError {
			return "", fmt.Errorf("host tool errored: %s", r.Output)
		}
		return r.Output, nil
	}
}

// routeCallback delivers a host reply to the goroutine blocked in
// CallHost. An unmatched id (a stale or duplicate reply) is dropped,
// matching the host's own behaviour for unknown ids.
func (s *session) routeCallback(f frame) {
	s.pendMu.Lock()
	ch, ok := s.pending[f.ID]
	s.pendMu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- callResp{result: f.Result, err: f.Error}:
	default:
	}
}

// writeFrame serialises one frame to the output under wmu so concurrent
// handlers (and callback requests) can never interleave bytes on the
// shared stdout. Each frame is newline-terminated and flushed.
func (s *session) writeFrame(f frame) {
	raw, err := json.Marshal(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sdk: encode frame:", err)
		return
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_, _ = s.w.Write(raw)
	_ = s.w.WriteByte('\n')
	_ = s.w.Flush()
}
