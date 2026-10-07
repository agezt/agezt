// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"crypto/subtle"
	"errors"
	graphs "github.com/agezt/agezt/kernel/workflow"
	"strings"
	"time"
)

const (
	RunTimeout          = 15 * time.Minute
	NodeTestTimeout     = 3 * time.Minute
	WebhookReplyTimeout = 2 * time.Minute
)

type Wake struct{ Source, Reason, ScheduleID, StandingID, StandingName, TriggerSubject, ParentCorrelation string }
type RunResult struct {
	Executed []string
	Outputs  map[string]any
}
type NodeResult struct {
	Output   any
	Port     string
	Attempts int
}
type ExecutionHost struct {
	Correlation func() string
	Run         func(context.Context, string, string, any) (RunResult, error)
	TestNode    func(context.Context, string, graphs.Workflow, string, map[string]any, any) (NodeResult, error)
	WithWake    func(context.Context, Wake) context.Context
	Detach      func(func())
	Panic       func(Wake, string, string, any)
}
type Execution struct {
	reader Reader
	host   ExecutionHost
}

func NewExecution(reader Reader, host ExecutionHost) *Execution {
	if host.Detach == nil {
		host.Detach = func(fn func()) { go fn() }
	}
	return &Execution{reader: reader, host: host}
}

type RunInput struct {
	Ref     string
	Payload any
	Async   bool
}

// Pointers distinguish absent variant fields from required null/false/empty ones.
type RunOutput struct {
	CorrelationID string          `json:"correlation_id"`
	Executed      *[]string       `json:"executed,omitempty"`
	Outputs       *map[string]any `json:"outputs,omitempty"`
	Accepted      *bool           `json:"accepted,omitempty"`
	Async         *bool           `json:"async,omitempty"`
	Workflow      *string         `json:"workflow,omitempty"`
}
type ExecutionError struct {
	Cause                 error
	CorrelationID, Prefix string
}

func (e ExecutionError) Error() string {
	return e.Prefix + e.Cause.Error() + " (correlation " + e.CorrelationID + ")"
}
func (e ExecutionError) Unwrap() error { return e.Cause }
func (s *Execution) wake(ctx context.Context, wake Wake) context.Context {
	if s.host.WithWake != nil {
		return s.host.WithWake(ctx, wake)
	}
	return ctx
}
func (s *Execution) Run(parent context.Context, in RunInput) (RunOutput, error) {
	if err := parent.Err(); err != nil {
		return RunOutput{}, err
	}
	corr := operationCorrelation(parent, "", s.host.Correlation)
	if in.Async {
		w, found := s.reader.Get(strings.TrimSpace(in.Ref))
		if !found {
			return RunOutput{}, UnknownWorkflowError{Ref: in.Ref}
		}
		s.host.Detach(func() { s.detached(context.WithoutCancel(parent), Wake{Source: "manual"}, corr, w.Name, in.Payload) })
		accepted, async := true, true
		return RunOutput{CorrelationID: corr, Accepted: &accepted, Async: &async, Workflow: &w.Name}, nil
	}
	ctx, cancel := context.WithTimeout(parent, RunTimeout)
	defer cancel()
	ctx = s.wake(ctx, Wake{Source: "manual"})
	res, err := s.host.Run(ctx, corr, in.Ref, in.Payload)
	if err != nil {
		if errors.Is(err, graphs.ErrNotFound) {
			return RunOutput{}, UnknownWorkflowError{Ref: in.Ref}
		}
		return RunOutput{}, ExecutionError{Cause: err, CorrelationID: corr}
	}
	return RunOutput{CorrelationID: corr, Executed: &res.Executed, Outputs: &res.Outputs}, nil
}
func (s *Execution) detached(parent context.Context, wake Wake, corr, name string, payload any) {
	defer func() {
		if value := recover(); value != nil && s.host.Panic != nil {
			s.host.Panic(wake, corr, name, value)
		}
	}()
	ctx, cancel := context.WithTimeout(parent, RunTimeout)
	defer cancel()
	ctx = s.wake(ctx, wake)
	_, _ = s.host.Run(ctx, corr, name, payload)
}

type NodeInput struct {
	Workflow graphs.Workflow
	Node     string
	Data     map[string]any
	Payload  any
}
type NodeOutput struct {
	Output        any    `json:"output"`
	Port          string `json:"port"`
	Attempts      int    `json:"attempts"`
	CorrelationID string `json:"correlation_id"`
}

func (s *Execution) TestNode(parent context.Context, in NodeInput) (NodeOutput, error) {
	if err := parent.Err(); err != nil {
		return NodeOutput{}, err
	}
	corr := operationCorrelation(parent, "", s.host.Correlation)
	ctx, cancel := context.WithTimeout(parent, NodeTestTimeout)
	defer cancel()
	res, err := s.host.TestNode(ctx, corr, in.Workflow, in.Node, in.Data, in.Payload)
	if err != nil {
		return NodeOutput{}, err
	}
	return NodeOutput{Output: res.Output, Port: res.Port, Attempts: res.Attempts, CorrelationID: corr}, nil
}

var ErrWebhookRefused = errors.New("webhook refused")

type WebhookInput struct {
	Ref, Secret string
	Invalid     bool
	Payload     any
}
type WebhookOutput struct {
	CorrelationID string          `json:"correlation_id"`
	Workflow      string          `json:"workflow"`
	Accepted      *bool           `json:"accepted,omitempty"`
	Executed      *[]string       `json:"executed,omitempty"`
	Outputs       *map[string]any `json:"outputs,omitempty"`
}

func (s *Execution) Webhook(parent context.Context, in WebhookInput) (WebhookOutput, error) {
	if err := parent.Err(); err != nil {
		return WebhookOutput{}, err
	}
	ref := strings.TrimSpace(in.Ref)
	if in.Invalid || ref == "" || in.Secret == "" {
		return WebhookOutput{}, ErrWebhookRefused
	}
	w, found := s.reader.Get(ref)
	if !found || !w.Enabled {
		return WebhookOutput{}, ErrWebhookRefused
	}
	spec := w.TriggerSpec()
	if spec.Kind != "webhook" || subtle.ConstantTimeCompare([]byte(spec.Secret), []byte(in.Secret)) != 1 {
		return WebhookOutput{}, ErrWebhookRefused
	}
	corr := operationCorrelation(parent, "", s.host.Correlation)
	wake := Wake{Source: "webhook", TriggerSubject: "webhook:" + w.Name}
	if spec.Reply {
		ctx, cancel := context.WithTimeout(parent, WebhookReplyTimeout)
		defer cancel()
		ctx = s.wake(ctx, wake)
		res, err := s.host.Run(ctx, corr, w.Name, in.Payload)
		if err != nil {
			return WebhookOutput{}, ExecutionError{Cause: err, CorrelationID: corr, Prefix: "webhook run failed: "}
		}
		return WebhookOutput{CorrelationID: corr, Workflow: w.Name, Executed: &res.Executed, Outputs: &res.Outputs}, nil
	}
	s.host.Detach(func() { s.detached(context.WithoutCancel(parent), wake, corr, w.Name, in.Payload) })
	accepted := true
	return WebhookOutput{CorrelationID: corr, Workflow: w.Name, Accepted: &accepted}, nil
}
