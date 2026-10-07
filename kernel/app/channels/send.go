// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Sender func(context.Context, string, string, string) error
type Outbound struct{ send Sender }

func NewOutbound(send Sender) *Outbound { return &Outbound{send: send} }

type SendInput struct{ Channel, To, Text string }
type SendOutput = map[string]any

// Send's terminal callback keeps the call context alive through native response
// delivery, preserving the measured legacy lifecycle during this move.
func (s *Outbound) Send(_ context.Context, in SendInput, reply func(SendOutput, error)) {
	kind := strings.ToLower(strings.TrimSpace(in.Channel))
	to := strings.TrimSpace(in.To)
	text := strings.TrimSpace(in.Text)
	if kind == "" || to == "" || text == "" {
		reply(nil, fmt.Errorf("send requires channel, to, and text"))
		return
	}
	if s.send == nil {
		reply(nil, fmt.Errorf("no channels configured (set a channel token to enable send)"))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.send(ctx, kind, to, text); err != nil {
		reply(nil, err)
		return
	}
	reply(SendOutput{"sent": true, "channel": kind, "to": to}, nil)
}
