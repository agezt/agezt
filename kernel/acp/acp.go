// SPDX-License-Identifier: MIT

package acp

// Package documentation lives in doc.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/platform/acpclient"
)

// New builds a Server reading JSON-RPC from in and writing to out.
func New(runner Runner, in io.Reader, out io.Writer) *Server {
	return &Server{
		runner:   runner,
		sc:       acpclient.NewBoundedScanner(in),
		out:      out,
		sessions: map[string]string{},
	}
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
		var req acpclient.Request
		if err := acpclient.ScanMessage(s.sc, &req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("acp: decode: %w", err)
		}
		s.dispatch(ctx, req)
	}
}

func (s *Server) dispatch(ctx context.Context, req acpclient.Request) {
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
		"protocolVersion": acpclient.ProtocolVersion,
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

func (s *Server) handleNewSession(params json.RawMessage) (any, *acpclient.RPCError) {
	var p newSessionParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &acpclient.RPCError{Code: codeInvalidParams, Message: "invalid params: " + err.Error()}
		}
	}
	s.sessM.Lock()
	defer s.sessM.Unlock()
	s.nextID++
	id := fmt.Sprintf("sess-%d", s.nextID)
	s.sessions[id] = p.Cwd
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

func (s *Server) handlePrompt(ctx context.Context, params json.RawMessage) (any, *acpclient.RPCError) {
	var p promptParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &acpclient.RPCError{Code: codeInvalidParams, Message: "invalid params: " + err.Error()}
	}
	var cwd string
	var ok bool
	func() {
		s.sessM.Lock()
		defer s.sessM.Unlock()
		cwd, ok = s.sessions[p.SessionID]
	}()
	if !ok {
		return nil, &acpclient.RPCError{Code: codeInvalidParams, Message: "unknown sessionId: " + p.SessionID}
	}

	intent := flattenPrompt(p.Prompt)
	if intent == "" {
		return nil, &acpclient.RPCError{Code: codeInvalidParams, Message: "empty prompt"}
	}

	streamed := false
	onChunk := func(kind ChunkKind, text string) {
		if text == "" {
			return
		}
		// Reasoning (M322) maps to ACP's agent_thought_chunk so editors render
		// it in their "thinking" UI, distinct from the answer. It does NOT set
		// `streamed` — that flag guards the non-streaming answer fallback, and a
		// run that emitted only reasoning still needs its answer delivered.
		update := "agent_message_chunk"
		if kind == ChunkThought {
			update = "agent_thought_chunk"
		} else {
			streamed = true
		}
		s.sendUpdate(p.SessionID, map[string]any{
			"sessionUpdate": update,
			"content":       map[string]any{"type": "text", "text": text},
		})
	}

	answer, err := s.runner.Prompt(ctx, cwd, intent, onChunk)
	if err != nil {
		return nil, &acpclient.RPCError{Code: codeInternal, Message: err.Error()}
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
func (s *Server) sendUpdate(sessionID string, update map[string]any) {
	s.writeMessage(acpclient.Notification{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params:  map[string]any{"sessionId": sessionID, "update": update},
	})
}

func (s *Server) reply(id json.RawMessage, result any) {
	if len(id) == 0 {
		return
	}
	s.writeMessage(acpclient.Response{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) replyOrError(id json.RawMessage, result any, rerr *acpclient.RPCError) {
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
	s.writeMessage(acpclient.Response{JSONRPC: "2.0", ID: id, Error: &acpclient.RPCError{Code: code, Message: msg}})
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
