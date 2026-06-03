// SPDX-License-Identifier: MIT

// Package sdk is the public Go client for embedding Agezt. It connects to a
// local Agezt daemon and runs intents through the same governed kernel loop
// that `agt run` uses — Edict policy, the journal, cost governance — without the
// caller depending on internal kernel packages or knowing the control-plane
// wire protocol.
//
// Usage:
//
//	c, err := sdk.Dial("") // "" → the default base ($AGEZT_HOME or ~/.agezt)
//	if err != nil {
//		log.Fatal(err)
//	}
//	res, err := c.Run(ctx, "summarise the repo", sdk.WithModel("claude-opus-4-8"))
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(res.Answer)
//
// To observe a run as it happens, use RunStream with an event callback.
package sdk

import (
	"context"
	"time"

	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
)

// Event is a journaled run event delivered to a RunStream callback. It aliases
// the kernel event type so callers get the full, stable event shape (Seq, Kind,
// Payload, …) without importing kernel internals.
type Event = event.Event

// DefaultBaseDir returns the daemon's base directory: $AGEZT_HOME if set,
// otherwise <user-home>/.agezt. Dial("") resolves the same value.
func DefaultBaseDir() (string, error) { return paths.BaseDir() }

// Client is a high-level client for a local Agezt daemon.
type Client struct {
	cp *controlplane.Client
}

// Dial connects to the local Agezt daemon recorded under baseDir. An empty
// baseDir resolves the default (see DefaultBaseDir). It reads the daemon's
// address and token from disk and does not require the daemon to be reachable
// until the first Run — so a Dial error means "no daemon recorded here", while a
// Run error can mean the daemon isn't responding.
func Dial(baseDir string) (*Client, error) {
	if baseDir == "" {
		b, err := paths.BaseDir()
		if err != nil {
			return nil, err
		}
		baseDir = b
	}
	cp, err := controlplane.NewClient(baseDir)
	if err != nil {
		return nil, err
	}
	return &Client{cp: cp}, nil
}

// Result is a completed run.
type Result struct {
	// Answer is the agent's final message.
	Answer string
	// CorrelationID identifies the run; pass it to `agt why` to walk the chain.
	CorrelationID string
	// Model is the model the run billed under (empty for an unpriced/mock run).
	Model string
	// Iterations is the number of agent loop turns (tool calls + final).
	Iterations int
	// CostUSD is the run's spend in dollars (0 when unpriced).
	CostUSD float64
}

// Option configures a single run.
type Option func(*runConfig)

type runConfig struct {
	model             string
	tenant            string
	system            string
	timeout           time.Duration
	tools             *[]string // nil = the full default toolset; non-nil = an explicit allow-list (may be empty)
	images            []string
	maxCostMicrocents int64
}

// WithModel overrides the model for this run (empty → the daemon's default).
func WithModel(m string) Option { return func(c *runConfig) { c.model = m } }

// WithTenant routes the run to an isolated tenant's kernel (requires the daemon
// started with multi-tenancy enabled).
func WithTenant(t string) Option { return func(c *runConfig) { c.tenant = t } }

// WithSystem replaces the base system prompt for this run.
func WithSystem(s string) Option { return func(c *runConfig) { c.system = s } }

// WithTimeout sets a per-run wall-clock timeout.
func WithTimeout(d time.Duration) Option { return func(c *runConfig) { c.timeout = d } }

// WithTools restricts the run to the named tools. Calling it with no names is an
// explicit empty allow-list (no tools) — distinct from not calling it at all,
// which leaves the full default toolset available.
func WithTools(names ...string) Option {
	return func(c *runConfig) {
		ns := append([]string{}, names...)
		c.tools = &ns
	}
}

// WithImages attaches image data: URLs (data:<media-type>;base64,<bytes>) to the
// run for a vision-capable model.
func WithImages(dataURLs ...string) Option {
	return func(c *runConfig) { c.images = append(c.images, dataURLs...) }
}

// WithMaxCostUSD caps the run's spend at the given dollar amount. A non-positive
// value is ignored (no cap).
func WithMaxCostUSD(usd float64) Option {
	return func(c *runConfig) {
		if usd > 0 {
			c.maxCostMicrocents = int64(usd * 1e9) // $1 = 1e9 microcents
		}
	}
}

// Run executes an intent and returns the final answer. It is RunStream with no
// event callback.
func (c *Client) Run(ctx context.Context, intent string, opts ...Option) (*Result, error) {
	return c.RunStream(ctx, intent, nil, opts...)
}

// RunStream executes an intent, invoking onEvent (if non-nil) for each journaled
// event as it streams, and returns the final Result. The run executes on the
// daemon under full governance; onEvent must not block for long (it runs on the
// stream-reading path).
func (c *Client) RunStream(ctx context.Context, intent string, onEvent func(*Event), opts ...Option) (*Result, error) {
	var cfg runConfig
	for _, o := range opts {
		o(&cfg)
	}
	cb := func(e *event.Event) {
		if onEvent != nil {
			onEvent(e)
		}
	}
	res, err := c.cp.Stream(ctx, controlplane.CmdRun, buildRunArgs(intent, cfg), cb)
	if err != nil {
		return nil, err
	}
	return parseResult(res), nil
}

// buildRunArgs renders a run config into the control-plane CmdRun argument map,
// byte-for-byte as the `agt run` CLI does (timeout as a Go duration string,
// tools/images as JSON arrays, max_cost as microcents).
func buildRunArgs(intent string, cfg runConfig) map[string]any {
	args := map[string]any{"intent": intent}
	if cfg.tenant != "" {
		args["tenant"] = cfg.tenant
	}
	if cfg.model != "" {
		args["model"] = cfg.model
	}
	if cfg.system != "" {
		args["system"] = cfg.system
	}
	if cfg.timeout > 0 {
		args["timeout"] = cfg.timeout.String()
	}
	if cfg.tools != nil {
		ts := make([]any, len(*cfg.tools))
		for i, n := range *cfg.tools {
			ts[i] = n
		}
		args["tools"] = ts
	}
	if len(cfg.images) > 0 {
		imgs := make([]any, len(cfg.images))
		for i, u := range cfg.images {
			imgs[i] = u
		}
		args["images"] = imgs
	}
	if cfg.maxCostMicrocents > 0 {
		args["max_cost"] = float64(cfg.maxCostMicrocents)
	}
	return args
}

// parseResult maps the CmdRun result object to a Result. JSON numbers decode to
// float64, which intFromAny normalises.
func parseResult(m map[string]any) *Result {
	r := &Result{}
	r.Answer, _ = m["answer"].(string)
	r.CorrelationID, _ = m["correlation_id"].(string)
	r.Model, _ = m["model"].(string)
	r.Iterations = int(intFromAny(m["iters"]))
	r.CostUSD = float64(intFromAny(m["spent_mc"])) / 1e9
	return r
}

func intFromAny(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}
