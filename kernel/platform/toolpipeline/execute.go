// SPDX-License-Identifier: MIT

package toolpipeline

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/toolinvoke"
)

// Execution preserves the backend outcome and the caller's timeout/panic facts.
// Terminal classification, scheduling, audit and hooks remain caller-owned.
type Execution struct {
	Result     toolapi.Result
	Err        error
	PanicValue any
	TimedOut   bool
}

// Execute runs one admitted tool under an optional positive per-call timeout.
// panicError lets a loop retain its terminal error identity, before context
// cleanup; nil retains the direct invoker's existing panic error. The deadline
// fact is captured before cancellation, including opaque backend errors.
func Execute(ctx context.Context, tool toolapi.Tool, input json.RawMessage, timeout time.Duration, panicError func(any) error) Execution {
	toolCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		toolCtx, cancel = context.WithTimeout(toolCtx, timeout)
	}
	res, panicValue, err := toolinvoke.Invoke(toolCtx, tool, input)
	outcome := Execution{Result: res, Err: err, PanicValue: panicValue}
	if panicValue != nil && panicError != nil {
		outcome.Err = panicError(panicValue)
	}
	outcome.TimedOut = timeout > 0 && toolCtx.Err() == context.DeadlineExceeded
	if cancel != nil {
		cancel()
	}
	return outcome
}
