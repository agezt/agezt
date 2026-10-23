// SPDX-License-Identifier: MIT

package plugin

// Plugin wire-format helpers: Invoke + InvokeWithProgress + startWaiter +
// Close + call + callWithProgress + writeRequest + writeFrame + readFrame.
// Carved out of host.go during the Day 33 god file split #1.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

func (p *Plugin) Invoke(ctx context.Context, name string, input json.RawMessage) (InvokeResult, error) {
	return p.InvokeWithProgress(ctx, name, input, nil)
}

// InvokeWithProgress is Invoke + per-call progress streaming (M1.ss).
// onProgress is called once per `{"progress":"..."}` notification the
// plugin emits against this request's id. Pass nil to drop progress
// silently (equivalent to Invoke).
//
// **Ordering guarantees.** Progress callbacks fire in the order the
// plugin emitted them, and all are guaranteed to complete BEFORE
// InvokeWithProgress returns its terminal result. This is the
// natural shape for "show the operator what's happening while
// the tool runs."
//
// **Backpressure / blocking.** The callback runs on the host read
// loop. A slow callback throttles further reads from the plugin
// (which then blocks on its stdout write — natural backpressure).
// Callers MUST NOT block indefinitely in cb; do any heavy work
// asynchronously off a channel you populate from the callback.
func (p *Plugin) InvokeWithProgress(
	ctx context.Context,
	name string,
	input json.RawMessage,
	onProgress func(string),
) (InvokeResult, error) {
	params, err := json.Marshal(InvokeParams{Name: name, Input: input})
	if err != nil {
		return InvokeResult{}, fmt.Errorf("plugin: marshal invoke params: %w", err)
	}
	callCtx, cancel := context.WithTimeout(ctx, p.cfg.InvokeTimeout)
	defer cancel()
	raw, err := p.callWithProgress(callCtx, MethodInvoke, params, onProgress)
	if err != nil {
		return InvokeResult{}, err
	}
	var out InvokeResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return InvokeResult{}, fmt.Errorf("plugin: parse invoke result: %w", err)
	}
	return out, nil
}

// startWaiter launches the single goroutine that owns cmd.Wait() for a started
// child, closing the returned channel once the process is reaped. It is what
// guarantees reaping on every death path — self-exit, crash, or kill — not only via
// Close (M422). Exactly one waiter exists per started process; nothing else may call
// cmd.Wait() (a second call would error/race).
func startWaiter(cmd *exec.Cmd) chan struct{} {
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	return done
}

// Close sends shutdown, gives the plugin DefaultShutdownGrace to
// exit, then kills it. Idempotent — Close on an already-dead
// plugin is a no-op.
func (p *Plugin) Close() error {
	p.mu.Lock()
	cmd := p.cmd
	stdin := p.stdin
	waitDone := p.waitDone
	p.mu.Unlock()
	alreadyDead := p.dead.Load()

	// Best-effort shutdown notification to a still-live plugin. If the write fails,
	// the process is already gone or unreachable — proceed to kill. Guard a nil
	// stdin so Close is safe on a Plugin that never finished starting (M183).
	if stdin != nil && !alreadyDead {
		_ = p.writeRequest(Request{ID: "end", Method: MethodShutdown})
	}

	// Ensure the child is stopped AND reaped on every path (M422). The dedicated
	// per-process waiter owns cmd.Wait(); here we only wait for it (giving a live
	// plugin the grace period) or force it via a process-group kill. Idempotent: a
	// second Close, or a Close after an abnormal markDead, finds waitDone already
	// closed and the kill a no-op — never a double Wait. Skip when there is no
	// started process (half-initialized Plugin, M183).
	if cmd != nil && cmd.Process != nil && waitDone != nil {
		if alreadyDead {
			// markDead doesn't kill (it would race a concurrent Reload swapping
			// p.cmd), so a plugin marked dead for an abnormal reason may still be
			// alive — force teardown now, then let the waiter reap.
			killProcessTree(cmd)
			<-waitDone
		} else {
			timer := time.NewTimer(DefaultShutdownGrace)
			defer timer.Stop()
			select {
			case <-waitDone:
			case <-timer.C:
				// Kill the whole process group (M184) so grandchildren are reaped too.
				killProcessTree(cmd)
				<-waitDone
			}
		}
	}
	p.dead.Store(true)
	p.setDeathErr(errors.New("plugin: closed"))
	// Drain pending; readers see "plugin dead" via the death sentinel.
	p.mu.Lock()
	for id, ch := range p.pending {
		close(ch)
		delete(p.pending, id)
	}
	p.mu.Unlock()
	return nil
}

// call sends a request and waits for the matching response. The
// id is minted from nextID. Returns the result bytes (or an error
// derived from a non-empty Response.Error). Equivalent to
// callWithProgress with a nil callback.
func (p *Plugin) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	return p.callWithProgress(ctx, method, params, nil)
}

// callWithProgress is call + optional per-request progress callback
// (M1.ss). onProgress is invoked once per `{"progress":"..."}`
// notification from the plugin matching this request's id. The
// callback is unregistered in the same defer that drops the
// pending channel, so a slow progress line that arrives after the
// terminal response is dropped (rather than racing the next call's
// re-used id).
func (p *Plugin) callWithProgress(
	ctx context.Context,
	method string,
	params json.RawMessage,
	onProgress func(string),
) (json.RawMessage, error) {
	if p.dead.Load() {
		return nil, fmt.Errorf("plugin: dead: %w", p.deathError())
	}
	id := "q-" + strconv.FormatInt(p.nextID.Add(1), 10)
	ch := make(chan *Response, 1)
	p.mu.Lock()
	// Re-check liveness under the lock. The check above is lock-free, so a Close or
	// markDead could mark the plugin dead and drain `pending` between it and here —
	// and a registration that lands AFTER that drain would never be closed, leaving
	// this caller blocked until its ctx deadline instead of failing fast (M464).
	// markDead/Close set dead and drain under this same lock, so checking here makes
	// registration and teardown mutually exclusive: either we register before the
	// drain (and the drain then closes our channel), or we see dead and bail.
	if p.dead.Load() {
		p.mu.Unlock()
		return nil, fmt.Errorf("plugin: dead: %w", p.deathError())
	}
	p.pending[id] = ch
	if onProgress != nil {
		p.progress[id] = onProgress
	}
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		delete(p.pending, id)
		delete(p.progress, id)
		p.mu.Unlock()
	}()

	req := Request{ID: id, Method: method, Params: params}
	if err := p.writeRequest(req); err != nil {
		return nil, err
	}
	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("plugin: connection lost: %w", p.deathError())
		}
		if resp.Error != "" {
			return nil, errors.New(resp.Error)
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("plugin: %s: %w", method, ctx.Err())
	}
}

func (p *Plugin) writeRequest(req Request) error {
	raw, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("plugin: marshal request: %w", err)
	}
	raw = append(raw, '\n')
	return p.writeFrame(raw, "request")
}

// writeFrame serializes a single newline-framed write to the child's stdin on
// writeMu so frames never interleave, but does NOT hold mu across the blocking
// pipe write — otherwise a plugin that floods stdout without draining its stdin
// would wedge the read loop (deliver needs mu) against the stuck writer (M460).
// stdin is snapshotted under mu because respawn swaps it.
func (p *Plugin) writeFrame(raw []byte, kind string) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	p.mu.Lock()
	w := p.stdin
	p.mu.Unlock()
	if w == nil {
		return fmt.Errorf("plugin: write %s: stdin closed", kind)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("plugin: write %s: %w", kind, err)
	}
	return nil
}

// inboundFrame is the union of Request and Response wire shapes.
// Each line on the plugin's stdout is parsed into this struct;
// the presence of `method` distinguishes a plugin→host callback
// (M1.cb) from a normal response to a host-initiated call.
type inboundFrame struct {
	ID       string          `json:"id"`
	Method   string          `json:"method,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
	Progress string          `json:"progress,omitempty"`
}

// readFrame reads one newline-delimited frame from r, bounding the
// total to max bytes (M177). It reads in buffer-sized chunks via
// ReadSlice (which returns bufio.ErrBufferFull when a line is longer
// than the reader's internal buffer); each chunk is copied out before
// the next read, so the returned slice is stable. Once the accumulated
// frame would exceed max, it returns errFrameTooLarge instead of
// allocating further — so an untrusted plugin that never emits '\n'
// (or emits a giant line) can't OOM the daemon. A trailing chunk with
// io.EOF (stream ended mid-line) is returned with that error, matching
// the prior ReadBytes('\n') behavior (the caller treats it as fatal).
