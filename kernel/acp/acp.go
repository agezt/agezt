// SPDX-License-Identifier: MIT

// Package acp implements an Agent Client Protocol server (SPEC-15 §3): Agezt
// as an agent backend that IDEs (Zed, and other ACP clients) drive over
// JSON-RPC 2.0 on stdio. The editor spawns the agent process, calls
// `initialize` → `session/new` → `session/prompt`, and receives streamed
// `session/update` notifications while the prompt runs.
//
// The protocol handling here is transport- and kernel-agnostic: it reads/writes
// JSON-RPC over any io.Reader/io.Writer and delegates the actual work to a
// Runner. The `agt acp` command wires a Runner backed by the control-plane
// client, so an ACP prompt runs through the same kernel tool-loop + Edict +
// journal as `agt run` — an editor driving Agezt does not bypass governance
// (SPEC-15 §3.3).
package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/agezt/agezt/internal/brand"
)

// ProtocolVersion is the ACP version this server implements.
const ProtocolVersion = 1

// Runner executes one prompt as an agent run. onChunk is called for each
// streamed text delta; the returned string is the full final answer (used to
// emit a single chunk when the provider did not stream). All work must pass
// through the kernel's governed path.
type Runner interface {
	Prompt(ctx context.Context, cwd, intent string, onChunk func(text string)) (string, error)
}

// Server speaks ACP over a reader/writer pair.
type Server struct {
	runner Runner
	dec    *json.Decoder
	out    io.Writer
	writeM sync.Mutex // serialize notifications vs responses on out

	sessM    sync.Mutex
	sessions map[string]string // sessionId -> cwd
	nextID   int
}

// New builds a Server reading JSON-RPC from in and writing to out.
func New(runner Runner, in io.Reader, out io.Writer) *Server {
	return &Server{
		runner:   runner,
		dec:      json.NewDecoder(in),
		out:      out,
		sessions: map[string]string{},
	}
}

// --- JSON-RPC wire types ---

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // absent for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

// Serve runs the read/dispatch loop until the input closes or ctx is done.
func (s *Server) Serve(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var req request
		if err := s.dec.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("acp: decode: %w", err)
		}
		s.dispatch(ctx, req)
	}
}

func (s *Server) dispatch(ctx context.Context, req request) {
	switch req.Method {
	case "initialize":
		s.reply(req.ID, s.handleInitialize(req.Params))
	case "session/new":
		res, rerr := s.handleNewSession(req.Params)
		s.replyOrError(req.ID, res, rerr)
	case "session/prompt":
		res, rerr := s.handlePrompt(ctx, req.Params)
		s.replyOrError(req.ID, res, rerr)
	case "session/cancel", "$/cancelNotification":
		// Notification (no id) — best-effort; per-prompt ctx handles the
		// actual cancellation when wired by the transport.
	default:
		if len(req.ID) > 0 {
			s.replyError(req.ID, codeMethodNotFound, "method not found: "+req.Method)
		}
	}
}

func (s *Server) handleInitialize(_ json.RawMessage) any {
	return map[string]any{
		"protocolVersion": ProtocolVersion,
		"agentCapabilities": map[string]any{
			"loadSession": false,
			"promptCapabilities": map[string]any{
				"image": false,
				"audio": false,
			},
		},
		// agentInfo is the agent's own identity reported to the IDE/client — the
		// product name and version, distinct from protocolVersion above. Sourced
		// from brand so it tracks the real release (was a stale "0.1.0" literal).
		"agentInfo": map[string]any{"name": brand.Binary, "version": brand.Version},
	}
}

type newSessionParams struct {
	Cwd string `json:"cwd"`
}

func (s *Server) handleNewSession(params json.RawMessage) (any, *rpcError) {
	var p newSessionParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{codeInvalidParams, "invalid params: " + err.Error()}
		}
	}
	s.sessM.Lock()
	s.nextID++
	id := fmt.Sprintf("sess-%d", s.nextID)
	s.sessions[id] = p.Cwd
	s.sessM.Unlock()
	return map[string]any{"sessionId": id}, nil
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type promptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []contentBlock `json:"prompt"`
}

func (s *Server) handlePrompt(ctx context.Context, params json.RawMessage) (any, *rpcError) {
	var p promptParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, "invalid params: " + err.Error()}
	}
	s.sessM.Lock()
	cwd, ok := s.sessions[p.SessionID]
	s.sessM.Unlock()
	if !ok {
		return nil, &rpcError{codeInvalidParams, "unknown sessionId: " + p.SessionID}
	}

	intent := flattenPrompt(p.Prompt)
	if intent == "" {
		return nil, &rpcError{codeInvalidParams, "empty prompt"}
	}

	streamed := false
	onChunk := func(text string) {
		if text == "" {
			return
		}
		streamed = true
		s.sendUpdate(p.SessionID, map[string]any{
			"sessionUpdate": "agent_message_chunk",
			"content":       map[string]any{"type": "text", "text": text},
		})
	}

	answer, err := s.runner.Prompt(ctx, cwd, intent, onChunk)
	if err != nil {
		return nil, &rpcError{codeInternal, err.Error()}
	}
	// Non-streaming providers emit no chunks — deliver the answer as one.
	if !streamed && answer != "" {
		s.sendUpdate(p.SessionID, map[string]any{
			"sessionUpdate": "agent_message_chunk",
			"content":       map[string]any{"type": "text", "text": answer},
		})
	}
	return map[string]any{"stopReason": "end_turn"}, nil
}

// flattenPrompt concatenates the text content blocks of a prompt into one
// intent string (non-text blocks — images/resources — are ignored at v1).
func flattenPrompt(blocks []contentBlock) string {
	var out string
	for _, b := range blocks {
		if b.Type == "text" || (b.Type == "" && b.Text != "") {
			if out != "" {
				out += "\n"
			}
			out += b.Text
		}
	}
	return out
}

// --- writers ---

func (s *Server) sendUpdate(sessionID string, update map[string]any) {
	s.writeMessage(notification{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params:  map[string]any{"sessionId": sessionID, "update": update},
	})
}

func (s *Server) reply(id json.RawMessage, result any) {
	if len(id) == 0 {
		return
	}
	s.writeMessage(response{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) replyOrError(id json.RawMessage, result any, rerr *rpcError) {
	if rerr != nil {
		s.replyError(id, rerr.Code, rerr.Message)
		return
	}
	s.reply(id, result)
}

func (s *Server) replyError(id json.RawMessage, code int, msg string) {
	if len(id) == 0 {
		return
	}
	s.writeMessage(response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}

// writeMessage serializes one JSON-RPC message as a single newline-terminated
// line (ndjson framing), serialized so notifications and responses never
// interleave mid-line.
func (s *Server) writeMessage(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.writeM.Lock()
	defer s.writeM.Unlock()
	_, _ = s.out.Write(append(b, '\n'))
}
