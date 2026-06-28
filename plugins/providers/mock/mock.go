// SPDX-License-Identifier: MIT

// Package mock is an offline Provider for tests and demos when no real LLM
// is available. It returns a scripted sequence of responses in order.
package mock

import (
	"context"
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
	// Responder, if set, computes the response from the request instead of
	// replaying the scripted list — so a demo/test can reflect the input (e.g.
	// echo how many image attachments the message carried, M93). Takes
	// precedence over the scripted responses when non-nil.
	Responder func(agent.CompletionRequest) agent.CompletionResponse
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
	if p.Responder != nil {
		r := p.Responder(req)
		return &r, nil
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
