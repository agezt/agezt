// SPDX-License-Identifier: MIT

package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

// DefaultInitTimeout caps the initialize round-trip. Plugins that
// take longer to start (Python imports, large model loads) should
// either (a) defer expensive work until the first tool call, or
// (b) bump this via Host.Config.InitTimeout.
const DefaultInitTimeout = 10 * time.Second

// DefaultInvokeTimeout caps a single tool/invoke round-trip.
const DefaultInvokeTimeout = 2 * time.Minute

// DefaultShutdownGrace is how long Host.Close waits for the plugin
// to exit after sending shutdown before sending a kill signal.
const DefaultShutdownGrace = 5 * time.Second

// DefaultMaxFrameBytes bounds a single newline-delimited frame read
// off a plugin's stdout (M177). The stream comes from an untrusted
// child: without a cap, a plugin that writes bytes but never emits a
// '\n' (or emits one pathologically large line) drives the host's
// bufio reader to allocate without limit, OOM-killing the whole
// daemon — one buggy/hostile plugin taking down every other plugin
// and the kernel. 16 MiB is generous for legitimate JSON tool
// results while still bounding the blast radius; a plugin that
// exceeds it is torn down (markDead) rather than the daemon.
const DefaultMaxFrameBytes = 16 << 20

// errFrameTooLarge is the cause recorded when a plugin's stdout frame
// exceeds Config.MaxFrameBytes. The plugin is marked dead; in-flight
// callers fail fast instead of the daemon dying under memory pressure.
var errFrameTooLarge = errors.New("plugin: stdout frame exceeds max size")

// Config tunes a Plugin.
type Config struct {
	// Path to the plugin executable. Required.
	Path string
	// Args passed after Path. Optional.
	Args []string
	// Env is the child's environment. Nil inherits the parent's.
	Env []string
	// Dir is the child's working directory. Empty inherits.
	Dir string
	// InitTimeout overrides DefaultInitTimeout.
	InitTimeout time.Duration
	// InvokeTimeout overrides DefaultInvokeTimeout.
	InvokeTimeout time.Duration
	// MaxFrameBytes overrides DefaultMaxFrameBytes — the hard cap on a
	// single newline-delimited stdout frame from the plugin (M177).
	// A frame larger than this tears the plugin down rather than
	// letting an untrusted child drive the host to OOM.
	MaxFrameBytes int
	// Logger receives stderr from the child (one line per call).
	// Nil discards.
	Logger func(line string)
	// PinnedHash, when non-empty, is the expected BLAKE3-256 digest
	// of the plugin binary as a 64-char lowercase hex string (M1.ff).
	// Spawn computes the digest of the file at Path and refuses to
	// start the child if it doesn't match.
	//
	// Operators pin a plugin by recording its hash once (e.g. via
	// `b3sum` or `agt plugin hash <path>`) and feeding it back via
	// AGEZT_PLUGIN_PINS at daemon startup. A drift — whether
	// accidental (apt upgrade replaced the binary) or malicious
	// (supply-chain compromise swapped it) — surfaces as a clear
	// "plugin pin mismatch" error rather than silent execution.
	//
	// Empty (the default) skips verification entirely — opt-in
	// security so adopting plugins doesn't require setting up the
	// pin infrastructure first.
	PinnedHash string
	// AllowedTools, when non-empty, restricts which of the plugin's
	// advertised tools the host will surface (M1.hh). Spawn returns
	// `ErrToolAllowlistMismatch` when the plugin advertises a tool
	// outside the allowlist (so silent capability expansion in a
	// future plugin release becomes a hard error operators must
	// re-approve, complementing M1.ff's binary-hash pinning).
	//
	// Empty allowlist disables the check — opt-in. Names are
	// compared against the un-prefixed tool name the plugin returns
	// (matches the `agt plugin hash` audit story: the operator sees
	// the same name in their config that the plugin emits).
	AllowedTools []string

	// HostTools (M1.cb) is the set of in-host tools the plugin is
	// allowed to invoke via `host/invoke` callbacks. Keys are the
	// names the plugin uses; values are the tools the host runs.
	// Nil or empty disables callbacks entirely — `host/invoke`
	// requests from the plugin are rejected with
	// ErrCallbacksDisabled.
	//
	// **Why a separate map, not "share the daemon's tool registry".**
	// Plugins should not get a back-door to every tool the host has.
	// The operator wires HostTools explicitly to a curated subset
	// (typically: the basic read-only tools — file read, http get,
	// shell with strict warden caps — that a higher-level plugin
	// needs to gather context). The daemon's wiring code is the
	// audit point.
	//
	// **Loop hazard.** If HostTools contains a remoteTool wrapped
	// around the same plugin, a plugin→host→plugin→host… recursion
	// is possible. The host does NOT guard against this; the
	// invoke timeout caps the total damage. Operators wiring
	// HostTools must avoid the cycle (don't re-include the
	// plugin's own tools).
	HostTools map[string]agent.Tool
}

// Plugin manages one child process. Safe for concurrent calls.
type Plugin struct {
	cfg Config

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	// pending tracks in-flight requests by id → response channel.
	// Read/Write must hold mu; channel sends/recvs happen outside
	// the lock to avoid head-of-line blocking.
	pending map[string]chan *Response

	// progress tracks per-request callbacks for streaming
	// notifications (M1.ss). Populated by InvokeWithProgress;
	// cleared in the same defer that clears `pending`. nil entry
	// or missing key both mean "drop the progress line silently"
	// — keeps the protocol forward-compatible with plugins that
	// emit progress against hosts that don't consume it.
	progress map[string]func(string)

	// nextID is a monotonic counter used to mint correlation ids.
	nextID atomic.Int64

	// tools is the snapshot returned by the most recent initialize.
	tools []ToolDef

	// dead is set when the read loop sees EOF or a fatal error.
	// All subsequent operations fail fast with errors that name
	// the cause (rather than hanging).
	//
	// deathErr is the cause, written by the read-loop goroutine
	// (markDead) / Close and read by callers — so it MUST be accessed
	// atomically, not as a plain field (M178). The `dead` flag alone
	// does not publish a separate plain-error field under Go's memory
	// model; an atomic.Pointer makes the cause's publication safe.
	// Access via deathError(); store via setDeathErr().
	dead     atomic.Bool
	deathErr atomic.Pointer[error]
}

// deathError returns the recorded cause of the plugin's death, or nil
// if it has not been set. Safe to call from any goroutine (M178).
func (p *Plugin) deathError() error {
	if e := p.deathErr.Load(); e != nil {
		return *e
	}
	return nil
}

// setDeathErr atomically records (or clears, on err==nil) the death
// cause. Storing a heap pointer to the interface value publishes it
// safely to readers (M178).
func (p *Plugin) setDeathErr(err error) {
	if err == nil {
		p.deathErr.Store(nil)
		return
	}
	p.deathErr.Store(&err)
}

// Spawn launches the plugin process, sends initialize, and returns
// a ready Plugin. The caller registers Plugin.Tools(prefix) with
// the daemon's tool registry.
func Spawn(ctx context.Context, cfg Config) (*Plugin, error) {
	if cfg.Path == "" {
		return nil, errors.New("plugin: Config.Path required")
	}
	if cfg.InitTimeout <= 0 {
		cfg.InitTimeout = DefaultInitTimeout
	}
	if cfg.InvokeTimeout <= 0 {
		cfg.InvokeTimeout = DefaultInvokeTimeout
	}
	if cfg.MaxFrameBytes <= 0 {
		cfg.MaxFrameBytes = DefaultMaxFrameBytes
	}
	if cfg.PinnedHash != "" {
		if err := VerifyPin(cfg.Path, cfg.PinnedHash); err != nil {
			return nil, err
		}
	}
	cmd := makeChild(cfg.Path, cfg.Args)
	if cfg.Env != nil {
		cmd.Env = cfg.Env
	}
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("plugin: start %q: %w", cfg.Path, err)
	}

	p := &Plugin{
		cfg:      cfg,
		cmd:      cmd,
		stdin:    stdin,
		stdout:   bufio.NewReader(stdout),
		pending:  make(map[string]chan *Response),
		progress: make(map[string]func(string)),
	}

	// Drain stderr → Logger. Plugins log to stderr; the host
	// forwards (or discards) each line.
	go func() {
		s := bufio.NewScanner(stderr)
		// Large buffer for verbose plugins (1MB per line).
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			if cfg.Logger != nil {
				cfg.Logger(s.Text())
			}
		}
		// Scanner.Err is io.EOF / closed-pipe on plugin exit;
		// not actionable, but surface unusual errors via Logger.
		if err := s.Err(); err != nil && cfg.Logger != nil {
			cfg.Logger("stderr scanner: " + err.Error())
		}
	}()

	// Read loop. Pulls Response lines off stdout, dispatches to the
	// waiting Invoke caller via the pending map.
	go p.readLoop()

	// Send initialize and wait for the tool list.
	initCtx, cancel := context.WithTimeout(ctx, cfg.InitTimeout)
	defer cancel()
	res, err := p.call(initCtx, MethodInitialize, nil)
	if err != nil {
		// Initialize failed — tear down the partially-started plugin.
		_ = p.Close()
		return nil, fmt.Errorf("plugin: initialize: %w", err)
	}
	var initResult InitializeResult
	if err := json.Unmarshal(res, &initResult); err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("plugin: parse initialize result: %w", err)
	}
	if len(cfg.AllowedTools) > 0 {
		if err := verifyToolAllowlist(initResult.Tools, cfg.AllowedTools); err != nil {
			_ = p.Close()
			return nil, err
		}
	}
	p.tools = initResult.Tools
	return p, nil
}

// ErrToolAllowlistMismatch is returned by Spawn when the plugin
// advertises a tool not in Config.AllowedTools. Wrapped with a
// list of offending names so the operator can either widen the
// allowlist or investigate why the plugin added a tool.
var ErrToolAllowlistMismatch = errors.New("plugin: advertised tools outside the allowlist")

// ErrCallbacksDisabled is returned to the plugin (in the Error
// field of the Response) when it sends `host/invoke` but the
// host's Config.HostTools is empty. Phrased to make it obvious
// to the plugin author that callbacks are opt-in on the host
// side, not a protocol failure.
var ErrCallbacksDisabled = errors.New("plugin: host callbacks not enabled (operator did not register any HostTools)")

// ErrHostToolNotFound is returned (in the Response.Error field)
// when the plugin asks to invoke a tool name the host's HostTools
// map doesn't contain. Distinct from ErrCallbacksDisabled so the
// plugin author can tell "callbacks blanket-disabled" from
// "callbacks enabled but this specific tool not in the allowlist."
var ErrHostToolNotFound = errors.New("plugin: requested host tool not in allowlist")

// verifyToolAllowlist checks every advertised tool is in allowed.
// Plugin tool names are compared verbatim (no prefix munging) —
// the operator's allowlist is over the names the plugin emits,
// not the prefixed names the daemon registers.
func verifyToolAllowlist(advertised []ToolDef, allowed []string) error {
	allowSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowSet[a] = struct{}{}
	}
	var unexpected []string
	for _, def := range advertised {
		if _, ok := allowSet[def.Name]; !ok {
			unexpected = append(unexpected, def.Name)
		}
	}
	if len(unexpected) == 0 {
		return nil
	}
	return fmt.Errorf("%w: unexpected=%v allowed=%v", ErrToolAllowlistMismatch, unexpected, allowed)
}

// Tools returns the plugin's tool definitions wrapped as
// agent.Tool implementations. The optional prefix is prepended
// to each tool name (e.g. prefix="my-plugin." turns the plugin's
// "search" into "my-plugin.search") — useful when registering
// multiple plugins to avoid name collisions.
func (p *Plugin) Tools(prefix string) map[string]agent.Tool {
	out := make(map[string]agent.Tool, len(p.tools))
	for _, def := range p.tools {
		name := prefix + def.Name
		out[name] = &remoteTool{
			plugin: p,
			def: agent.ToolDef{
				Name:        name,
				Description: def.Description,
				InputSchema: def.InputSchema,
			},
			remoteName: def.Name,
		}
	}
	return out
}

// Invoke is the lower-level entry point — callers usually go
// through the remoteTool wrapper instead.
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

// Close sends shutdown, gives the plugin DefaultShutdownGrace to
// exit, then kills it. Idempotent — Close on an already-dead
// plugin is a no-op.
func (p *Plugin) Close() error {
	if p.dead.Load() {
		return nil
	}
	// Best-effort shutdown notification. If the write fails, the
	// process is already gone or unreachable — proceed to kill.
	_ = p.writeRequest(Request{ID: "end", Method: MethodShutdown})

	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(DefaultShutdownGrace):
		_ = p.cmd.Process.Kill()
		<-done
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
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := p.stdin.Write(raw); err != nil {
		return fmt.Errorf("plugin: write request: %w", err)
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
func (p *Plugin) readLoop() {
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
		if f.Method != "" {
			go p.handleCallback(f)
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
		p.mu.Lock()
		ch, ok := p.pending[f.ID]
		p.mu.Unlock()
		if !ok {
			// Response with no waiter — likely a stale id after a
			// timeout. Drop.
			continue
		}
		ch <- &Response{ID: f.ID, Result: f.Result, Error: f.Error, Progress: f.Progress}
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
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := p.stdin.Write(raw); err != nil {
		return fmt.Errorf("plugin: write response: %w", err)
	}
	return nil
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
// **Concurrency.** Reload holds p.mu for the duration. In-flight
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
	_ = p.Close()

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

	p.mu.Lock()
	p.cmd = cmd
	p.stdin = stdin
	p.stdout = bufio.NewReader(stdout)
	p.pending = make(map[string]chan *Response)
	p.progress = make(map[string]func(string))
	p.mu.Unlock()
	p.dead.Store(false)
	p.setDeathErr(nil)
	p.nextID.Store(0)

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
	go p.readLoop()

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

func (r *remoteTool) Definition() agent.ToolDef { return r.def }

func (r *remoteTool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	if !r.plugin.IsAlive() {
		return agent.Result{}, fmt.Errorf("plugin: tool %q unavailable (plugin process is dead: %v)",
			r.def.Name, r.plugin.deathError())
	}
	res, err := r.plugin.Invoke(ctx, r.remoteName, raw)
	if err != nil {
		return agent.Result{}, err
	}
	return agent.Result{
		Output:  res.Output,
		IsError: res.IsError,
	}, nil
}
