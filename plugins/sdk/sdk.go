// SPDX-License-Identifier: MIT

// SDK: Result + Text + Errorf + Handler + Tool + wire types + fromContext + Serve + ServeRW + Emit + CallHost (public SDK surface).
// Code extracted from sdk.go during the Day-136 god-file split.
// Public API unchanged.
package sdk


import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"encoding/json"
	"sync/atomic"
)


// maxFrameBytes bounds a single newline-delimited frame the SDK reads
// from the host. Mirrors kernel/plugin's DefaultMaxFrameBytes (16 MiB).
// bufio.Reader.ReadBytes/ReadSlice grow a single allocation until they
// see '\n'; without a cap, a host that writes a frame with no terminating
// newline — a corrupted pipe, or a partial write that never completes —
// would make the plugin allocate without bound until it is OOM-killed.
// 16 MiB is generous for legitimate JSON tool I/O.
const maxFrameBytes = 16 << 20

// readFrame reads one newline-terminated frame, capped at max bytes. An
// over-cap frame returns an error rather than allocating without bound;
// the caller treats it as terminal (clean exit) the same way the host
// marks an over-cap plugin dead. Mirrors kernel/plugin.readFrame.
func readFrame(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(buf)+len(chunk) > max {
			return nil, fmt.Errorf("sdk: host frame exceeds %d bytes", max)
		}
		buf = append(buf, chunk...)
		if err == bufio.ErrBufferFull {
			continue // line longer than the bufio buffer; keep reading
		}
		return buf, err
	}
}

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
	// Capability optionally declares which of the kernel's policy axes this
	// tool belongs to (M900) — e.g. "http.post", "file.write", "shell" — so
	// the operator's trust level and hard-deny rules for that axis apply to
	// this tool exactly like a built-in. Empty keeps the historical
	// classification; a value the kernel doesn't recognise is ignored.
	Capability string
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
	Capability  string          `json:"capability,omitempty"` // M900: declared policy axis
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
		line, err := readFrame(dec, maxFrameBytes)
		if err != nil {
			// EOF, read error, or an over-cap frame: the host went
			// away or is misbehaving. Clean exit (an over-cap frame
			// leaves the stream desynced, so we must not keep reading).
			return nil
		}
		// A correct host never writes a blank line; skip stray ones so
		// they don't produce a spurious empty-id error frame the host
		// can't correlate.
		if len(bytes.TrimSpace(line)) == 0 {
			continue
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

