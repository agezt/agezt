// SPDX-License-Identifier: MIT

// Package notify is the agent's proactive-messaging tool (M143). It lets a
// running agent send a short message to the operator over a configured channel
// (Telegram/Slack/Discord) MID-run — "I've started the long task, I'll report
// back", a progress note, or an alert — instead of staying silent until the
// final reply. This is the Jarvis "keep me posted" capability.
//
// Security (SPEC-04 §1.7): outbound from an agent is an injection-adjacent
// surface — a prompt-injected agent must not be able to message arbitrary
// recipients (exfiltration / spam). So the destinations are PINNED to the
// operator's own pre-configured allowlist: the agent supplies only the text (and
// optionally which channel kind), never the recipient id. The tool can therefore
// only ever talk to the operator's own chats. Gated by Edict CapNotify (allowed
// by default; an operator can raise or deny it). The send is journaled as
// channel.outbound by the underlying channel.
//
// Lifecycle: the daemon needs the live channels (built after the kernel) to wire
// the sender, but the tool must be in the kernel's tool map BEFORE the kernel —
// and its HTTP servers/channels — start, so the map is never written while the
// agent loop reads it (which would be a fatal concurrent-map race). So the tool
// is constructed unbound and registered up front, then Bind wires the sender
// once channels exist. Bind/Invoke synchronize on a mutex, so a run that somehow
// races boot sees either the unbound state (a clean "not configured" error) or
// the bound state, never a torn read.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/agent"
)

// Sender delivers text out a named channel kind to a specific channel/chat id.
// The daemon wires it to the live channels' Send methods.
type Sender func(ctx context.Context, kind, channelID, text string) error

// Tool implements agent.Tool. Constructed unbound via New; the daemon calls Bind
// once the live channels exist. Until bound, Invoke returns a clean error.
type Tool struct {
	mu      sync.RWMutex
	send    Sender
	targets map[string][]string // channel kind → operator's allowlisted ids
}

// New returns an unbound notify Tool. Call Bind before runs begin to wire it to
// the live channels.
func New() *Tool { return &Tool{} }

// Bind wires the tool to the channel sender and the operator's per-kind allowlist
// ids. Kinds with no ids are dropped. Safe to call once at boot; synchronized
// against Invoke. A nil send or empty targets leaves the tool effectively
// disabled (Invoke reports it's not configured).
func (t *Tool) Bind(send Sender, targets map[string][]string) {
	pruned := map[string][]string{}
	for kind, ids := range targets {
		if len(ids) > 0 {
			pruned[kind] = append([]string(nil), ids...)
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.send = send
	t.targets = pruned
}

// snapshot returns the current binding under a read lock.
func (t *Tool) snapshot() (Sender, map[string][]string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.send, t.targets
}

// kinds returns the configured channel kinds, sorted.
func kinds(targets map[string][]string) []string {
	ks := make([]string, 0, len(targets))
	for k := range targets {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func (t *Tool) Definition() agent.ToolDef {
	_, targets := t.snapshot()
	avail := strings.Join(kinds(targets), ", ")
	if avail == "" {
		avail = "(none configured yet)"
	}
	return agent.ToolDef{
		Name: "notify",
		Description: "Proactively send a short message to the operator over a configured chat channel " +
			"(" + avail + ") — e.g. progress on a long task, or an alert. " +
			"The message goes ONLY to the operator's pre-configured chats; you cannot choose arbitrary " +
			"recipients. Use sparingly, for things worth interrupting for.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "text": {
      "type": "string",
      "description": "The message to send to the operator."
    },
    "channel": {
      "type": "string",
      "description": "Optional: restrict delivery to one channel kind (telegram|slack|discord). Omit to send to all configured channels."
    }
  },
  "required": ["text"]
}`),
	}
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	send, targets := t.snapshot()
	if send == nil || len(targets) == 0 {
		return agent.Result{Output: "notify is not configured (no channel with an allowlist)", IsError: true}, nil
	}

	var in struct {
		Text    string `json:"text"`
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return agent.Result{Output: "text is required", IsError: true}, nil
	}

	// Resolve which channel kinds to deliver to.
	var deliver []string
	if k := strings.ToLower(strings.TrimSpace(in.Channel)); k != "" {
		if _, ok := targets[k]; !ok {
			return agent.Result{Output: fmt.Sprintf("channel %q is not configured; available: %s", k, strings.Join(kinds(targets), ", ")), IsError: true}, nil
		}
		deliver = []string{k}
	} else {
		deliver = kinds(targets)
	}

	sent := 0
	var errs []string
	for _, kind := range deliver {
		for _, id := range targets[kind] {
			if err := send(ctx, kind, id, text); err != nil {
				errs = append(errs, fmt.Sprintf("%s/%s: %v", kind, id, err))
				continue
			}
			sent++
		}
	}

	if sent == 0 {
		return agent.Result{Output: "notify failed: " + strings.Join(errs, "; "), IsError: true}, nil
	}
	out := fmt.Sprintf("notified the operator (%d recipient(s) across %s)", sent, strings.Join(deliver, ", "))
	if len(errs) > 0 {
		// Partial failure: surface it as an error result so the model (and any
		// automation keying on IsError) doesn't treat a half-delivered alert as
		// fully sent — the dangerous case for "I'll report back" messaging.
		return agent.Result{Output: out + "; but some deliveries FAILED: " + strings.Join(errs, "; "), IsError: true}, nil
	}
	return agent.Result{Output: out}, nil
}
