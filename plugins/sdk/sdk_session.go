// SPDX-License-Identifier: MIT

// SDK: session.initPayload + session.dispatchInvoke + session.runHandler + session.routeCallback + session.writeFrame (private session methods).
// Code extracted from sdk.go during the Day-136 god-file split.
// Public API unchanged.
package sdk


import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"encoding/json"
)

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
			Capability:  t.Capability, // M900
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
	return invokeResult(out)
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
