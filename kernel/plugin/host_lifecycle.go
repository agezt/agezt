// SPDX-License-Identifier: MIT

// Plugin host: Reload + respawn (lifecycle management).
// Code extracted from host_loop.go during the Day-144 god-file split.
// Public API unchanged.
package plugin


import (
	"bufio"
	"context"
	"fmt"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
)

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

