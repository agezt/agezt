// SPDX-License-Identifier: MIT

package plugin

// Plugin read-loop + callback dispatch + lifecycle reload/respawn:
// readLoop + deliver + dispatchCallback + handleCallback + rejectCallback +
// writeResponse + markDead + IsAlive + Reload + respawn. Carved out of
// host.go during the Day 33 god file split #1.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/agezt/agezt/kernel/agent"
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
func (p *Plugin) IsAlive() bool { return !p.dead.Load() }

// Reload swaps the underlying child process IN PLACE: the existing
// child is terminated (shutdown + grace + kill), a fresh one is
// spawned with the same Config, and the new tool list replaces the
// cached one (M1.qq).
//
// **Why in-place mutation.** Existing remoteTool wrappers (returned
// by Tools()) hold a *Plugin pointer; if Reload created a *new*
// Plugin, every cached wrapper would silently keep referencing the
// dead instance. In-place mutation means wrappers keep working
// across reloads — at worst, an Invoke for a tool the new plugin
// no longer advertises gets a "no such tool" error from the plugin
// itself, which is the right failure mode.
//
// **Pin + allowlist verification re-runs.** A reload is the right
// moment to re-check both: a redeployed plugin binary might have
// drifted from its pin (same threat as initial spawn), and a new
// initialize result might list extra tools outside the allowlist.
// Reload returns an error and leaves the OLD plugin running if
// either check fails — operators get a clean rollback rather than
// a half-reloaded daemon.
//
// **Concurrency.** Reload does NOT hold p.mu for its whole duration
// (it can't: respawn's own initialize round-trip acquires p.mu).
// Instead, Close marks the old plugin dead before respawn installs
// fresh state, so a caller racing the reload either observes
// dead==true and fails fast, or observes the live new child. The
// correlation-id counter (p.nextID) stays monotonic across the swap
// (M180), so even a late response from the old child cannot satisfy
// a new request. In-flight
// Invoke calls on the old child either complete (response arrives
// before shutdown processes) or fail with the death sentinel —
// either is observable to the caller, and the new child is
// already accepting requests by the time Reload returns.
func (p *Plugin) Reload(ctx context.Context) error {
	// Step 1: verify the binary STILL matches the pin and allowlist
	// before tearing the old child down. A failed pre-flight check
	// means we keep the old child running — failure-safe.
	if p.cfg.PinnedHash != "" {
		if err := VerifyPin(p.cfg.Path, p.cfg.PinnedHash); err != nil {
			return fmt.Errorf("plugin reload: %w", err)
		}
	}

	// Step 2: shut the existing child down. Best-effort — even on
	// failure, proceed to spawn the replacement (the old child is
	// still going to be killed by Close's grace timer).
	p.mu.Lock()
	oldReadDone := p.readDone
	p.mu.Unlock()
	_ = p.Close()
	// Join the old read loop before respawn reuses the struct (M560). Close reaps
	// the process, which shuts the old stdout pipe, so the loop's readFrame errors
	// and it returns promptly. Without this wait its trailing markDead ("read
	// stdout: file already closed") could land AFTER respawn resets p.dead/
	// deathErr — marking the brand-new child dead and failing initialize with a
	// phantom "connection lost". Bounded: the loop always closes its channel via
	// defer, even on panic.
	if oldReadDone != nil {
		<-oldReadDone
	}

	// Step 3: spawn a replacement using the same config. We bypass
	// the package-level Spawn function so we can mutate `p` in place
	// rather than returning a fresh struct.
	if err := p.respawn(ctx); err != nil {
		return fmt.Errorf("plugin reload: respawn: %w", err)
	}
	return nil
}

// respawn replaces the in-flight process with a fresh one and
// reruns initialize + (optional) allowlist verification. Called by
// Reload; not exported because the lifecycle is messy enough that
// callers should always go through Reload.
func (p *Plugin) respawn(ctx context.Context) error {
	cmd := makeChild(p.cfg.Path, p.cfg.Args)
	if p.cfg.Env != nil {
		cmd.Env = p.cfg.Env
	}
	if p.cfg.Dir != "" {
		cmd.Dir = p.cfg.Dir
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	// Dedicated waiter for the replacement child (M422). The previous child was
	// already reaped by Reload→Close before this point.
	waitDone := startWaiter(cmd)
	readDone := make(chan struct{})
	p.mu.Lock()
	p.cmd = cmd
	p.stdin = stdin
	p.stdout = bufio.NewReader(stdout)
	p.pending = make(map[string]chan *Response)
	p.progress = make(map[string]func(string))
	p.waitDone = waitDone
	p.readDone = readDone
	p.mu.Unlock()
	p.dead.Store(false)
	p.setDeathErr(nil)
	// Deliberately do NOT reset p.nextID (M180): the correlation-id
	// counter stays monotonic ACROSS reloads so an id is never reused.
	// Resetting to 0 made post-reload ids (q-1, q-2, …) collide with
	// pre-reload ones, so a late or crafted response carrying a reused
	// id could satisfy the wrong request (response confusion). A
	// monotonic counter makes that structurally impossible.

	// Stderr forwarder + read loop, mirroring Spawn.
	go func() {
		s := bufio.NewScanner(stderr)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			if p.cfg.Logger != nil {
				p.cfg.Logger(s.Text())
			}
		}
		if err := s.Err(); err != nil && p.cfg.Logger != nil {
			p.cfg.Logger("stderr scanner: " + err.Error())
		}
	}()
	go p.readLoop(readDone)

	initCtx, cancel := context.WithTimeout(ctx, p.cfg.InitTimeout)
	defer cancel()
	res, err := p.call(initCtx, MethodInitialize, nil)
	if err != nil {
		_ = p.Close()
		return fmt.Errorf("initialize: %w", err)
	}
	var initResult InitializeResult
	if err := json.Unmarshal(res, &initResult); err != nil {
		_ = p.Close()
		return fmt.Errorf("parse initialize result: %w", err)
	}
	if err := capAdvertisedTools(initResult.Tools, p.cfg.MaxAdvertisedTools); err != nil {
		_ = p.Close()
		return err
	}
	if len(p.cfg.AllowedTools) > 0 {
		if err := verifyToolAllowlist(initResult.Tools, p.cfg.AllowedTools); err != nil {
			_ = p.Close()
			return err
		}
	}
	p.tools = initResult.Tools
	return nil
}

// ----- remoteTool: bridges plugin tools into agent.Tool -----

type remoteTool struct {
	plugin     *Plugin
	def        agent.ToolDef
	remoteName string // name as the plugin knows it (no prefix)
}

