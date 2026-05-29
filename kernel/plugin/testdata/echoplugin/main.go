// SPDX-License-Identifier: MIT

// Command echoplugin is a minimal reference implementation of the
// agezt plugin protocol. Built and exec'd by the host test suite
// (see TestSpawn_InitializeAndInvoke). Also useful as a template
// for plugin authors writing their first agezt plugin.
//
// Protocol: read one Request JSON per line on stdin, write one
// Response per line on stdout. Methods supported:
//
//   - initialize → returns one tool definition ("echo")
//   - tool/invoke (name="echo") → returns the input echoed back
//     in the Output field
//   - tool/invoke (name="fail") → returns IsError=true with the
//     input as the error text (exercises the error path)
//   - shutdown → exits 0
//
// The plugin does NOT import any agezt package. The wire format
// is the entire contract — operators can write plugins in any
// language by following the same on-disk JSON shape.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
)

// inboundFrame is the union of Request (from host) and Response
// (from host, in reply to a host/invoke callback the plugin
// originated). The presence of `method` distinguishes them — same
// trick the host-side readLoop uses.
type inboundFrame struct {
	ID       string          `json:"id"`
	Method   string          `json:"method,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
	Progress string          `json:"progress,omitempty"`
}

type request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type response struct {
	ID       string          `json:"id"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
	Progress string          `json:"progress,omitempty"`
}

// Pending callbacks the plugin originated (M1.cb): keyed by the
// plugin-minted id; the response from the host gets routed to the
// channel via the demux loop.
var (
	writeMu  sync.Mutex // serialises stdout writes
	pendMu   sync.Mutex
	pending  = map[string]chan response{}
	cbNextID atomic.Int64
)

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

func main() {
	dec := bufio.NewReader(os.Stdin)
	enc := bufio.NewWriter(os.Stdout)
	defer enc.Flush()

	for {
		line, err := dec.ReadBytes('\n')
		if err != nil {
			return
		}
		var f inboundFrame
		if err := json.Unmarshal(line, &f); err != nil {
			writeResp(enc, response{ID: f.ID, Error: "bad request: " + err.Error()})
			continue
		}

		// Response to a callback we originated (M1.cb): route to
		// waiter. Frame has no Method field, but its id matches
		// the one we minted in callHost.
		if f.Method == "" {
			pendMu.Lock()
			ch, ok := pending[f.ID]
			pendMu.Unlock()
			if ok {
				ch <- response{ID: f.ID, Result: f.Result, Error: f.Error}
				continue
			}
			// Otherwise: unsolicited response with no waiter. Drop
			// silently — matches what the host does for stale ids.
			continue
		}

		// Inbound request from host.
		req := request{ID: f.ID, Method: f.Method, Params: f.Params}
		switch req.Method {
		case "initialize":
			r := initResult{Tools: []toolDef{{
				Name:        "echo",
				Description: "Echoes the input back as Output.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
			}, {
				Name:        "fail",
				Description: "Always returns IsError=true (for error-path testing).",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			}, {
				Name:        "slowwork",
				Description: "Emits three progress notifications then returns.",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			}, {
				Name:        "callhost",
				Description: "Calls the host's 'double' tool with the input and returns the doubled string (M1.cb fixture).",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
			}}}
			raw, _ := json.Marshal(r)
			writeResp(enc, response{ID: req.ID, Result: raw})

		case "tool/invoke":
			var p invokeParams
			if err := json.Unmarshal(req.Params, &p); err != nil {
				writeResp(enc, response{ID: req.ID, Error: "bad params: " + err.Error()})
				continue
			}
			switch p.Name {
			case "echo":
				out := invokeResult{Output: "echo: " + string(p.Input)}
				raw, _ := json.Marshal(out)
				writeResp(enc, response{ID: req.ID, Result: raw})
			case "fail":
				out := invokeResult{Output: "deliberate failure", IsError: true}
				raw, _ := json.Marshal(out)
				writeResp(enc, response{ID: req.ID, Result: raw})
			case "slowwork":
				// Stream three progress notifications, then a terminal
				// result. Exercises the M1.ss progress path.
				writeResp(enc, response{ID: req.ID, Progress: "step 1 of 3"})
				writeResp(enc, response{ID: req.ID, Progress: "step 2 of 3"})
				writeResp(enc, response{ID: req.ID, Progress: "step 3 of 3"})
				out := invokeResult{Output: "done"}
				raw, _ := json.Marshal(out)
				writeResp(enc, response{ID: req.ID, Result: raw})
			case "callhost":
				// M1.cb fixture: ask the host to invoke its "double"
				// tool with our input, then return whatever the host
				// produced. Concurrency note — we run the callback in
				// this goroutine (the read-loop goroutine) for
				// simplicity; in production a plugin should spawn
				// per-call goroutines so the read loop stays
				// responsive while callbacks are in flight.
				go func(reqID string, input json.RawMessage) {
					hostOut, err := callHost(enc, "double", input)
					if err != nil {
						writeResp(enc, response{ID: reqID, Error: err.Error()})
						return
					}
					out := invokeResult{Output: "via host: " + hostOut}
					raw, _ := json.Marshal(out)
					writeResp(enc, response{ID: reqID, Result: raw})
				}(req.ID, p.Input)
			default:
				writeResp(enc, response{ID: req.ID, Error: "unknown tool: " + p.Name})
			}

		case "shutdown":
			return

		default:
			writeResp(enc, response{ID: req.ID, Error: "unknown method: " + req.Method})
		}
	}
}

// callHost sends a host/invoke request and blocks until the host
// replies. Returns the Output string of the host tool's result.
// Errors include both transport-level problems (write failed) and
// the host's own Error field (tool not allowed / tool failed).
func callHost(enc *bufio.Writer, toolName string, input json.RawMessage) (string, error) {
	id := "cb-" + strconv.FormatInt(cbNextID.Add(1), 10)
	ch := make(chan response, 1)
	pendMu.Lock()
	pending[id] = ch
	pendMu.Unlock()
	defer func() {
		pendMu.Lock()
		delete(pending, id)
		pendMu.Unlock()
	}()

	params, _ := json.Marshal(invokeParams{Name: toolName, Input: input})
	writeReq(enc, request{ID: id, Method: "host/invoke", Params: params})

	resp := <-ch
	if resp.Error != "" {
		return "", fmt.Errorf("host returned error: %s", resp.Error)
	}
	var r invokeResult
	if err := json.Unmarshal(resp.Result, &r); err != nil {
		return "", fmt.Errorf("parse host result: %w", err)
	}
	if r.IsError {
		return "", fmt.Errorf("host tool errored: %s", r.Output)
	}
	return r.Output, nil
}

// writeReq sends a plugin→host Request frame. Held under writeMu
// so a callback request can't interleave bytes with a normal
// Response frame.
func writeReq(w *bufio.Writer, r request) {
	raw, err := json.Marshal(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, "encode request:", err)
		return
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	_, _ = w.Write(raw)
	_ = w.WriteByte('\n')
	_ = w.Flush()
}

func writeResp(w *bufio.Writer, r response) {
	raw, err := json.Marshal(r)
	if err != nil {
		// Should never happen.
		fmt.Fprintln(os.Stderr, "encode response:", err)
		return
	}
	// Hold the same lock writeReq uses so a callback request can't
	// interleave bytes with a normal response — both share stdout.
	writeMu.Lock()
	defer writeMu.Unlock()
	_, _ = w.Write(raw)
	_ = w.WriteByte('\n')
	_ = w.Flush()
}
