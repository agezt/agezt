// SPDX-License-Identifier: MIT

package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
)

// Approval is a pending human-in-the-loop request: the agent asked to use a
// capability that policy gates on explicit approval.
type Approval struct {
	// ID identifies the request; pass it to Approve or Deny.
	ID string
	// Capability is the gated capability (e.g. "shell.exec", "http.fetch").
	Capability string
	// Tool is the tool that triggered the request.
	Tool string
	// Reason is the policy reason the request was raised.
	Reason string
	// Actor is the run/agent that asked.
	Actor string
	// Input is the tool input, JSON-encoded when structured.
	Input string
	// Timeout is when the request auto-resolves (zero if it doesn't).
	Timeout time.Time
}

// PendingApprovals lists the human-in-the-loop requests currently awaiting a
// decision. It does not start a run.
func (c *Client) PendingApprovals(ctx context.Context) ([]Approval, error) {
	res, err := c.cp.Call(ctx, controlplane.CmdApprovals, nil)
	if err != nil {
		return nil, err
	}
	return parseApprovals(res), nil
}

// Approve grants a pending request by id, unblocking the run. reason is
// journaled and may be empty.
func (c *Client) Approve(ctx context.Context, id, reason string) error {
	return c.decide(ctx, id, "grant", reason)
}

// Deny rejects a pending request by id, failing the gated tool call. reason is
// journaled and may be empty.
func (c *Client) Deny(ctx context.Context, id, reason string) error {
	return c.decide(ctx, id, "deny", reason)
}

func (c *Client) decide(ctx context.Context, id, decision, reason string) error {
	_, err := c.cp.Call(ctx, controlplane.CmdDecide, map[string]any{
		"id": id, "decision": decision, "reason": reason,
	})
	return err
}

// parseApprovals maps the CmdApprovals result ({"pending":[…]}) to typed
// Approval values.
func parseApprovals(res map[string]any) []Approval {
	pending, _ := res["pending"].([]any)
	out := make([]Approval, 0, len(pending))
	for _, raw := range pending {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		a := Approval{}
		a.ID, _ = m["id"].(string)
		a.Capability, _ = m["capability"].(string)
		a.Tool, _ = m["tool_name"].(string)
		a.Reason, _ = m["reason"].(string)
		a.Actor, _ = m["actor"].(string)
		a.Input = anyToString(m["input"])
		if ts := intFromAny(m["timeout_unix"]); ts > 0 {
			a.Timeout = time.Unix(ts, 0)
		}
		out = append(out, a)
	}
	return out
}

// anyToString renders a decoded JSON value as a string: a string verbatim, a
// structured value re-encoded as compact JSON, nil as "".
func anyToString(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", v)
	}
}
