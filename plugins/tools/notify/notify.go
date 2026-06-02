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
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)

// Sender delivers text out a named channel kind to a specific channel/chat id.
// The daemon wires it to the live channels' Send methods.
type Sender func(ctx context.Context, kind, channelID, text string) error

// Tool implements agent.Tool. Constructed only when at least one channel has a
// non-empty allowlist (see New); otherwise the tool is not registered.
type Tool struct {
	send    Sender
	targets map[string][]string // channel kind → operator's allowlisted ids
}

// New builds a notify Tool. targets maps each channel kind to the operator's
// configured recipient ids; kinds with no ids are dropped. Returns nil when no
// kind has any target (nothing to notify → tool disabled).
func New(send Sender, targets map[string][]string) *Tool {
	pruned := map[string][]string{}
	for kind, ids := range targets {
		if len(ids) > 0 {
			pruned[kind] = ids
		}
	}
	if send == nil || len(pruned) == 0 {
		return nil
	}
	return &Tool{send: send, targets: pruned}
}

// kinds returns the configured channel kinds, sorted.
func (t *Tool) kinds() []string {
	ks := make([]string, 0, len(t.targets))
	for k := range t.targets {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "notify",
		Description: "Proactively send a short message to the operator over a configured chat channel " +
			"(" + strings.Join(t.kinds(), ", ") + ") — e.g. progress on a long task, or an alert. " +
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
	var kinds []string
	if k := strings.ToLower(strings.TrimSpace(in.Channel)); k != "" {
		if _, ok := t.targets[k]; !ok {
			return agent.Result{Output: fmt.Sprintf("channel %q is not configured; available: %s", k, strings.Join(t.kinds(), ", ")), IsError: true}, nil
		}
		kinds = []string{k}
	} else {
		kinds = t.kinds()
	}

	sent := 0
	var errs []string
	for _, kind := range kinds {
		for _, id := range t.targets[kind] {
			if err := t.send(ctx, kind, id, text); err != nil {
				errs = append(errs, fmt.Sprintf("%s/%s: %v", kind, id, err))
				continue
			}
			sent++
		}
	}

	if sent == 0 {
		return agent.Result{Output: "notify failed: " + strings.Join(errs, "; "), IsError: true}, nil
	}
	out := fmt.Sprintf("notified the operator (%d recipient(s) across %s)", sent, strings.Join(kinds, ", "))
	if len(errs) > 0 {
		out += "; some deliveries failed: " + strings.Join(errs, "; ")
	}
	return agent.Result{Output: out}, nil
}
