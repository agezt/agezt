// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"fmt"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"strings"
	"time"
)

type Sender func(context.Context, string, string, string) error
type Outbound struct{ send Sender }

func NewOutbound(send Sender) *Outbound { return &Outbound{send: send} }

type SendInput struct{ Channel, To, Text string }
type SendOutput struct {
	Sent    bool   `json:"sent"`
	Channel string `json:"channel"`
	To      string `json:"to"`
}

// Send's terminal callback keeps the call context alive through native response
// delivery, preserving the measured legacy lifecycle during this move.
// Send transfers optional cleanup ownership only after the sender returns.
// With no terminal owner the original direct callback lifetime is unchanged.
func (s *Outbound) Send(parent context.Context, in SendInput, reply func(SendOutput, error)) {
	kind := strings.ToLower(strings.TrimSpace(in.Channel))
	to := strings.TrimSpace(in.To)
	text := strings.TrimSpace(in.Text)
	if kind == "" || to == "" || text == "" {
		reply(SendOutput{}, fmt.Errorf("send requires channel, to, and text"))
		return
	}
	if s.send == nil {
		reply(SendOutput{}, fmt.Errorf("no channels configured (set a channel token to enable send)"))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	retained := false
	defer func() {
		if !retained {
			cancel()
		}
	}()
	err := s.send(ctx, kind, to, text)
	retained = opapi.DeferTerminalCleanup(parent, cancel)
	if err != nil {
		reply(SendOutput{}, err)
		return
	}
	reply(SendOutput{Sent: true, Channel: kind, To: to}, nil)
}
