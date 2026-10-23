// SPDX-License-Identifier: MIT

package acpagent

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
)

// wirePeer is a minimal ACP peer that speaks the JSON-RPC wire directly.
//
// The plugin's package must not reach the daemon's ACP SERVER (kernel/acp is
// an inbound adapter — a plugin may reach the contract and the platform
// layers, nothing else), so the fake external agent here answers the wire
// itself instead of borrowing the server. The proof that the real client and
// the real server interoperate lives in the server's own package
// (kernel/acp/roundtrip_test.go), which may reach the client.
type wirePeer struct {
	// chunks are streamed back as agent_message_chunk updates.
	chunks []string
	// answer is the stop reason returned with the prompt result.
	stopReason string
	// promptErr, when set, is answered as a JSON-RPC error for session/prompt.
	promptErr string
	// gotTask and gotCwd record what the peer was asked.
	gotTask string
	gotCwd  string
}

// serve answers the client's three requests until the input closes.
func (p *wirePeer) serve(in io.Reader, out io.Writer) {
	br := bufio.NewReader(in)
	for {
		line, err := br.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &req); err != nil {
			return
		}
		switch req.Method {
		case "initialize":
			p.write(out, `{"jsonrpc":"2.0","id":`+string(req.ID)+`,"result":{"protocolVersion":1}}`)
		case "session/new":
			var params struct {
				Cwd string `json:"cwd"`
			}
			_ = json.Unmarshal(req.Params, &params)
			p.gotCwd = params.Cwd
			p.write(out, `{"jsonrpc":"2.0","id":`+string(req.ID)+`,"result":{"sessionId":"sess-1"}}`)
		case "session/prompt":
			var params struct {
				SessionID string `json:"sessionId"`
				Prompt    []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"prompt"`
			}
			_ = json.Unmarshal(req.Params, &params)
			for _, b := range params.Prompt {
				p.gotTask += b.Text
			}
			if p.promptErr != "" {
				p.write(out, `{"jsonrpc":"2.0","id":`+string(req.ID)+`,"error":{"code":-32603,"message":`+jsonString(p.promptErr)+`}}`)
				continue
			}
			for _, c := range p.chunks {
				p.write(out, `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"`+params.SessionID+
					`","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":`+jsonString(c)+`}}}}`)
			}
			stop := p.stopReason
			if stop == "" {
				stop = "end_turn"
			}
			p.write(out, `{"jsonrpc":"2.0","id":`+string(req.ID)+`,"result":{"stopReason":"`+stop+`"}}`)
		}
		if err != nil {
			return
		}
	}
}

// write emits one newline-delimited JSON-RPC message.
func (p *wirePeer) write(out io.Writer, msg string) {
	_, _ = io.WriteString(out, msg+"\n")
}

// jsonString quotes a string as a JSON literal.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// fakePeer returns a dialFunc that wires the tool's ACP client to a wirePeer
// (the fake external agent) over a pair of pipes.
func fakePeer(p *wirePeer) dialFunc {
	return func(_ context.Context, _, _ string) (*transport, error) {
		toolToPeerR, toolToPeerW := io.Pipe()
		peerToToolR, peerToToolW := io.Pipe()
		go p.serve(toolToPeerR, peerToToolW)
		return &transport{
			out:   peerToToolR, // agent stdout → tool reads
			in:    toolToPeerW, // tool writes → agent stdin
			close: func() error { _ = toolToPeerW.Close(); return nil },
		}, nil
	}
}

// fakeCatalog is a stand-in for the ACP catalogue port.
type fakeCatalog struct {
	installed []string
	commands  map[string]string
}

func (c fakeCatalog) AnyInstalled() bool { return len(c.installed) > 0 }

func (c fakeCatalog) InstalledSlugs() []string { return c.installed }

func (c fakeCatalog) ResolveCommand(ref, fallback string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		fallback = strings.TrimSpace(fallback)
		return fallback, fallback != ""
	}
	cmd, ok := c.commands[ref]
	if !ok {
		return "", false
	}
	return cmd, true
}
