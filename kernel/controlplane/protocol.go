// SPDX-License-Identifier: MIT

package controlplane

// Package documentation lives in doc.go.

import "github.com/agezt/agezt/kernel/event"

// Command names supported by the control plane.
type Request struct {
	ID    string         `json:"id"`
	Cmd   string         `json:"cmd"`
	Token string         `json:"token"`
	Args  map[string]any `json:"args,omitempty"`
}

// Response types.
const (
	RespEvent  = "event"
	RespResult = "result"
	RespError  = "error"
)

// Response is the wire shape sent by the server. Exactly one of Event,
// Result, or Error is populated depending on Type.
type Response struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Event     *event.Event   `json:"event,omitempty"`
	Result    map[string]any `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	ErrorCode string         `json:"error_code,omitempty"` // optional domain classification on errors
}

// File names under <BaseDir>/runtime/.
const (
	addrFile  = "control.addr"
	tokenFile = "control.token"
)
