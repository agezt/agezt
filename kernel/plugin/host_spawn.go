// SPDX-License-Identifier: MIT

package plugin

// Plugin spawn + tool advertisement helpers: deathError + setDeathErr +
// Spawn + checkProtocolVersion + capAdvertisedTools + verifyToolAllowlist +
// Tools + ToolCapabilities. Carved out of host.go during the Day 33 god
// file split #1.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/agent"
)

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
	if cfg.MaxConcurrentCallbacks <= 0 {
		cfg.MaxConcurrentCallbacks = DefaultMaxConcurrentCallbacks
	}
	if cfg.MaxAdvertisedTools <= 0 {
		cfg.MaxAdvertisedTools = DefaultMaxAdvertisedTools
	}
	// Resolve a bare-name path the SAME way exec will, so the pin hash and the
	// executed binary are the identical file (M422). See resolvePluginPath.
	cfg.Path = resolvePluginPath(cfg.Path)
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
		cbSem:    make(chan struct{}, cfg.MaxConcurrentCallbacks),
	}
	// Dedicated waiter: reap the child on whatever path it dies (M422).
	p.waitDone = startWaiter(cmd)
	readDone := make(chan struct{})
	p.readDone = readDone

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
	go p.readLoop(readDone)

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
	if err := checkProtocolVersion(initResult.ProtocolVersion); err != nil {
		_ = p.Close()
		return nil, err
	}
	if err := capAdvertisedTools(initResult.Tools, cfg.MaxAdvertisedTools); err != nil {
		_ = p.Close()
		return nil, err
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

// ErrProtocolVersionMismatch is returned by Spawn when the plugin's
// advertised protocol_version differs from the host's ProtocolVersion.
// A major mismatch means the wire contract changed; the host rejects
// the plugin rather than failing cryptically mid-run.
var ErrProtocolVersionMismatch = errors.New("plugin: protocol version mismatch")

// checkProtocolVersion enforces that the plugin speaks a compatible wire
// protocol version. Plugins that omit the field (version 0) default to 1
// for backward compatibility with plugins written before this field existed.
func checkProtocolVersion(pluginVersion int) error {
	if pluginVersion == 0 {
		pluginVersion = 1 // back-compat: missing field = v1
	}
	if pluginVersion != ProtocolVersion {
		return fmt.Errorf("%w: plugin advertises v%d, host speaks v%d",
			ErrProtocolVersionMismatch, pluginVersion, ProtocolVersion)
	}
	return nil
}

// capAdvertisedTools fails when a plugin advertises more tools than the
// configured maximum (M182), so a hostile initialize result can't blow
// up the registry. Returns a wrapped ErrTooManyTools naming the count.
func capAdvertisedTools(advertised []ToolDef, max int) error {
	if len(advertised) > max {
		return fmt.Errorf("%w: %d > %d", ErrTooManyTools, len(advertised), max)
	}
	return nil
}

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
				Effect: agent.ToolEffect{
					Class: agent.EffectCompensable,
					PredictedEffects: []string{
						"Forward this call to an operator-installed plugin process.",
					},
					AffectedResources: []string{"plugin process " + p.cfg.Path, "resources reachable by plugin tool " + def.Name},
					RollbackNotes:     "Compensation depends on the plugin implementation and declared capability; stop or reload the plugin to prevent future calls.",
					Confidence:        0.45,
				},
			},
			remoteName: def.Name,
		}
	}
	return out
}

// ToolCapabilities returns the DECLARED capability per prefixed tool name
// (M900): only tools whose manifest carried a non-empty Capability appear.
// The kernel validates each declaration against its known capability set and
// feeds the survivors to the policy engine, so a plugin tool can join an
// existing policy axis (and its trust level / hard-deny rules) instead of
// classifying as an unknown one-off.
func (p *Plugin) ToolCapabilities(prefix string) map[string]string {
	out := map[string]string{}
	for _, def := range p.tools {
		if def.Capability != "" {
			out[prefix+def.Name] = def.Capability
		}
	}
	return out
}

// Invoke is the lower-level entry point — callers usually go
// through the remoteTool wrapper instead.
