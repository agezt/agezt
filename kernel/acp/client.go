// SPDX-License-Identifier: MIT

package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// Client drives an external ACP agent (SPEC-15 §3, the inverse of Server): it
// speaks ACP JSON-RPC 2.0 as the *client* — initialize → session/new →
// session/prompt — over the agent process's stdio, relaying the agent's
// streamed agent_message_chunk updates. Used by the acp-agent bridge tool so a
// Agezt run can orchestrate another ACP agent as a step.
//
// Usage is synchronous: one call at a time on a single goroutine.
type Client struct {
	enc    *json.Encoder // to the agent's stdin
	dec    *json.Decoder // from the agent's stdout
	nextID int
}

// NewClient wires a Client to an agent's streams: agentOut is the agent's
// stdout (we read responses/notifications), agentIn is its stdin (we write
// requests).
func NewClient(agentOut io.Reader, agentIn io.Writer) *Client {
	return &Client{enc: json.NewEncoder(agentIn), dec: json.NewDecoder(agentOut)}
}

// Initialize negotiates the protocol version. The agent's capabilities are
// currently ignored (we use only prompt streaming).
func (c *Client) Initialize(ctx context.Context) error {
	_, err := c.call(ctx, "initialize", map[string]any{"protocolVersion": ProtocolVersion}, nil)
	return err
}

// NewSession opens a session rooted at cwd and returns its id.
func (c *Client) NewSession(ctx context.Context, cwd string) (string, error) {
	res, err := c.call(ctx, "session/new", map[string]any{"cwd": cwd, "mcpServers": []any{}}, nil)
	if err != nil {
		return "", err
	}
	var r struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return "", err
	}
	if r.SessionID == "" {
		return "", fmt.Errorf("acp client: agent returned empty sessionId")
	}
	return r.SessionID, nil
}

// Prompt sends a text prompt and relays the agent's streamed message chunks to
// onChunk, returning the stopReason when the prompt completes.
func (c *Client) Prompt(ctx context.Context, sessionID, text string, onChunk func(string)) (string, error) {
	params := map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	}
	res, err := c.call(ctx, "session/prompt", params, func(method string, p json.RawMessage) {
		if method != "session/update" || onChunk == nil {
			return
		}
		var upd struct {
			Update struct {
				SessionUpdate string `json:"sessionUpdate"`
				Content       struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"update"`
		}
		if json.Unmarshal(p, &upd) == nil && upd.Update.SessionUpdate == "agent_message_chunk" {
			if upd.Update.Content.Text != "" {
				onChunk(upd.Update.Content.Text)
			}
		}
	})
	if err != nil {
		return "", err
	}
	var r struct {
		StopReason string `json:"stopReason"`
	}
	_ = json.Unmarshal(res, &r)
	return r.StopReason, nil
}

// call sends a request and reads messages until the matching response arrives,
// passing any interleaved notifications to onNotify. Returns the result bytes.
func (c *Client) call(ctx context.Context, method string, params any, onNotify func(method string, params json.RawMessage)) (json.RawMessage, error) {
	c.nextID++
	id := c.nextID
	if err := c.enc.Encode(request{JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", id)), Method: method, Params: mustRaw(params)}); err != nil {
		return nil, fmt.Errorf("acp client: write %s: %w", method, err)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if err := c.dec.Decode(&msg); err != nil {
			return nil, fmt.Errorf("acp client: read %s: %w", method, err)
		}
		// Notification (no id, has method) → relay and keep reading.
		if len(msg.ID) == 0 && msg.Method != "" {
			if onNotify != nil {
				onNotify(msg.Method, msg.Params)
			}
			continue
		}
		// A response: check it's ours.
		if string(msg.ID) != fmt.Sprintf("%d", id) {
			continue // not our response (shouldn't happen with synchronous use)
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("acp client: %s: %s (code %d)", method, msg.Error.Message, msg.Error.Code)
		}
		return msg.Result, nil
	}
}

func mustRaw(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
