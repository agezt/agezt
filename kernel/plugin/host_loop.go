// SPDX-License-Identifier: MIT

// Plugin host: readLoop + deliver + dispatchCallback + handleCallback + rejectCallback + writeResponse + markDead + IsAlive (live-streaming loop).
// Code extracted from host_loop.go during the Day-144 god-file split.
// Public API unchanged.
package plugin


import (
	"bufio"
	"context"
	"fmt"

	"encoding/json"
)


func readFrame(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(buf)+len(chunk) > max {
			return nil, errFrameTooLarge
		}
		buf = append(buf, chunk...)
		if err == bufio.ErrBufferFull {
			continue // line longer than the bufio buffer; keep reading
		}
		return buf, err
	}
}

// readLoop pulls one frame per line off stdout, routes responses
// to the waiting goroutine via the pending map, and dispatches
// plugin-initiated host/invoke requests (M1.cb). Runs until EOF /
// error. On exit, marks the plugin dead so subsequent calls fail
// fast instead of blocking on the pending channel forever.
func (p *Plugin) readLoop(done chan struct{}) {
	// Signal exit so Reload can join this goroutine before respawn reuses the
	// struct (M560). Closes the channel it was STARTED with — never the current
	// p.readDone field, which a concurrent respawn may already have replaced.
	defer close(done)
	// Defense-in-depth (M179): the read loop processes untrusted plugin
	// output. Any unforeseen panic here must tear the plugin down, not
	// crash the whole daemon — mark it dead so callers fail fast.
	defer func() {
		if r := recover(); r != nil {
			p.markDead(fmt.Errorf("plugin: read loop panic: %v", r))
		}
	}()
	for {
		line, err := readFrame(p.stdout, p.cfg.MaxFrameBytes)
		if err != nil {
			p.markDead(fmt.Errorf("read stdout: %w", err))
			return
		}
		var f inboundFrame
		if err := json.Unmarshal(line, &f); err != nil {
			// Don't kill the plugin over one bad line — the next
			// line might be fine. But the in-flight caller for
			// whatever id this was supposed to satisfy will time
			// out on the context deadline.
			if p.cfg.Logger != nil {
				p.cfg.Logger(fmt.Sprintf("plugin: bad response line: %v", err))
			}
			continue
		}

		// Plugin-initiated callback (M1.cb): `method` field set.
		// Dispatch on a goroutine so a slow host tool doesn't block
		// the read loop from receiving the plugin's other replies.
		// The dispatcher writes its own Response back via writeRequest-
		// equivalent (writeResponse).
		//
		// Bounded fan-out (M181): acquire a callback slot (non-blocking)
		// before spawning. A full semaphore means MaxConcurrentCallbacks
		// are already in flight, so we reject this one inline rather than
		// spawn an unbounded goroutine — keeps the read loop responsive
		// and goroutines bounded under a host/invoke flood. The slot is
		// released in handleCallback's defer.
		if f.Method != "" {
			p.dispatchCallback(f)
			continue
		}

		// Progress notification (M1.ss): forward to the callback
		// without consuming the pending channel. Multiple progress
		// lines per request are fine — the channel is only spent
		// when the terminal response arrives.
		//
		// **Synchronous dispatch.** We deliberately call cb on the
		// read-loop goroutine so progress is observed in arrival
		// order AND lands before the terminal response unblocks
		// the Invoke caller. A pathologically-slow callback will
		// throttle further reads from the plugin (which then
		// blocks on its stdout write — natural backpressure).
		// Callers must not block indefinitely in cb; the doc on
		// InvokeWithProgress states this.
		if f.Progress != "" && f.Result == nil && f.Error == "" {
			p.mu.Lock()
			cb := p.progress[f.ID]
			p.mu.Unlock()
			if cb != nil {
				cb(f.Progress)
			}
			continue
		}
		p.deliver(f)
	}
}

// deliver routes a terminal response frame to its waiting caller. The
// lookup AND the send happen UNDER p.mu, and the send is non-blocking
// (M179). markDead/Close close pending channels under the same lock and
// delete the id in the same critical section, so deliver and teardown
// are mutually exclusive: the read loop can never send on a channel a
// concurrent teardown just closed (which would panic this goroutine
// and, unrecovered, crash the daemon). The channel is buffered (cap 1)
// and single-use, so the one legitimate response always fits; a hostile
// plugin that double-sends a terminal frame for one id hits `default`
// and is dropped rather than blocking the loop while holding mu. A
// frame with no waiter (stale id after a timeout) is silently dropped.
func (p *Plugin) deliver(f inboundFrame) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch, ok := p.pending[f.ID]
	if !ok {
		return
	}
	select {
	case ch <- &Response{ID: f.ID, Result: f.Result, Error: f.Error, Progress: f.Progress}:
	default:
		// Buffer already full (duplicate terminal frame) — drop.
	}
}

// dispatchCallback decides how to handle a plugin-initiated callback
// frame under the concurrency cap (M181). It acquires a slot from the
// bounded cbSem non-blockingly: on success it spawns handleCallback
// (which releases the slot on exit); when the semaphore is full —
// MaxConcurrentCallbacks already in flight — it rejects the callback
// inline with ErrTooManyCallbacks instead of spawning an unbounded
// goroutine. Keeping the acquire non-blocking means a host/invoke flood
// can never stall the read loop nor exhaust goroutines/memory.
func (p *Plugin) dispatchCallback(f inboundFrame) {
	select {
	case p.cbSem <- struct{}{}:
		go p.handleCallback(f)
	default:
		p.rejectCallback(f, ErrTooManyCallbacks)
	}
}

// handleCallback runs one plugin→host invoke (M1.cb). Routes to
// the configured HostTools map; returns either the tool's output
// or an error in the Response.Error field. Runs on its own
// goroutine so the read loop never blocks waiting for a host tool.
//
// Method dispatch is hardcoded to MethodHostInvoke for now — the
// plugin protocol has no other plugin-originated method in v1.
// Unknown methods get a clear error rather than silent drop so
// the plugin author sees the typo.
func (p *Plugin) handleCallback(f inboundFrame) {
	// Release the callback slot acquired by the dispatcher (M181). This
	// defer runs last (registered first), after the response is written.
	defer func() { <-p.cbSem }()
	resp := Response{ID: f.ID}
	defer func() {
		if err := p.writeResponse(resp); err != nil && p.cfg.Logger != nil {
			p.cfg.Logger(fmt.Sprintf("plugin: write callback response: %v", err))
		}
	}()

	if f.Method != MethodHostInvoke {
		resp.Error = fmt.Sprintf("plugin: unknown plugin-initiated method %q (only %q supported in v1)",
			f.Method, MethodHostInvoke)
		return
	}
	if len(p.cfg.HostTools) == 0 {
		resp.Error = ErrCallbacksDisabled.Error()
		return
	}
	var params InvokeParams
	if err := json.Unmarshal(f.Params, &params); err != nil {
		resp.Error = fmt.Sprintf("plugin: bad host/invoke params: %v", err)
		return
	}
	tool, ok := p.cfg.HostTools[params.Name]
	if !ok {
		resp.Error = fmt.Sprintf("%v: %q", ErrHostToolNotFound, params.Name)
		return
	}

	// Bound the callback the same way Invoke bounds tool/invoke
	// in the other direction — the operator's InvokeTimeout caps
	// both. Without this, a plugin could weave a tool that loops
	// forever on the host side.
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.InvokeTimeout)
	defer cancel()
	res, err := tool.Invoke(ctx, params.Input)
	if err != nil {
		resp.Error = err.Error()
		return
	}
	out, err := json.Marshal(InvokeResult{Output: res.Output, IsError: res.IsError})
	if err != nil {
		resp.Error = fmt.Sprintf("plugin: marshal callback result: %v", err)
		return
	}
	resp.Result = out
}

// rejectCallback writes an error Response for a callback the host
// declined without running it (M181) — currently the over-capacity
// case. Called inline on the read-loop goroutine (no goroutine spawned,
// no semaphore slot held), so it must stay cheap: a single small write
// via the stdin mutex. The plugin sees the error on its callback id
// exactly as if a host tool had failed.
func (p *Plugin) rejectCallback(f inboundFrame, cause error) {
	resp := Response{ID: f.ID, Error: cause.Error()}
	if err := p.writeResponse(resp); err != nil && p.cfg.Logger != nil {
		p.cfg.Logger(fmt.Sprintf("plugin: write callback rejection: %v", err))
	}
}

// writeResponse sends a Response back to the plugin. Used by the
// callback dispatcher (M1.cb) when the host has just executed a
// host/invoke on behalf of the plugin. Concurrency-safe via the
// same stdin mutex writeRequest uses.
func (p *Plugin) writeResponse(resp Response) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("plugin: marshal response: %w", err)
	}
	raw = append(raw, '\n')
	return p.writeFrame(raw, "response")
}

// markDead is called by readLoop on terminal errors. It records
// the cause, marks the plugin dead, and closes every pending
// channel so blocked callers unblock with a clear error.
func (p *Plugin) markDead(cause error) {
	if !p.dead.CompareAndSwap(false, true) {
		return
	}
	p.setDeathErr(cause)
	p.mu.Lock()
	for id, ch := range p.pending {
		close(ch)
		delete(p.pending, id)
	}
	p.mu.Unlock()
}

// IsAlive reports whether the plugin process is still
// usable. False after Close, EOF, or a fatal stdio error.
