// SPDX-License-Identifier: MIT

// Package mock is an offline Provider for tests and demos when no real LLM
// is available. It returns a scripted sequence of responses in order.
package mock

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/agezt/agezt/kernel/agent"
)

// Provider replays a scripted sequence of completion responses. Useful for
// driving the agent loop deterministically in tests and for offline demos
// when ANTHROPIC_API_KEY is absent.
type Provider struct {
	mu        sync.Mutex
	responses []agent.CompletionResponse
	idx       int
	// OnRequest, if set, is called with each incoming request before the
	// scripted response is returned. Useful for test assertions.
	OnRequest func(agent.CompletionRequest)
}

// New returns a Provider that replays responses in order.
func New(responses ...agent.CompletionResponse) *Provider {
	return &Provider{responses: append([]agent.CompletionResponse{}, responses...)}
}

// Name implements agent.Provider.
func (p *Provider) Name() string { return "mock" }

// ErrExhausted is returned when Complete is called after the scripted
// responses have been exhausted.
var ErrExhausted = errors.New("mock: scripted responses exhausted")

// Complete implements agent.Provider.
func (p *Provider) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.OnRequest != nil {
		p.OnRequest(req)
	}
	if p.idx >= len(p.responses) {
		return nil, ErrExhausted
	}
	r := p.responses[p.idx]
	p.idx++
	return &r, nil
}

// CallCount returns how many times Complete has been called.
func (p *Provider) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.idx
}

// FinalText is a convenience constructor for a response that ends the loop
// with the given assistant text.
func FinalText(text string) agent.CompletionResponse {
	return agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: text},
		StopReason: agent.StopEndTurn,
	}
}

// ToolUse is a convenience constructor for a response that asks the loop to
// invoke one tool. input is JSON-marshaled.
func ToolUse(callID, toolName string, input any) agent.CompletionResponse {
	raw, err := json.Marshal(input)
	if err != nil {
		// In test code; panic is fine and surfaces typos fast.
		panic("mock.ToolUse: marshal input: " + err.Error())
	}
	return agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{{ID: callID, Name: toolName, Input: raw}},
		},
		StopReason: agent.StopToolUse,
	}
}
