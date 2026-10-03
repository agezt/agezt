// SPDX-License-Identifier: MIT

// Package toolinvoke provides panic-contained execution of an admitted tool.
// Callers own lookup, schema, policy, timeout and audit orchestration.
package toolinvoke

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// Invoke calls the tool once, preserving its result, error and context. A panic
// becomes the direct invoker's existing error text. panicValue additionally lets
// a loop retain its own run-terminal error classification without re-invoking.
func Invoke(ctx context.Context, tool toolapi.Tool, args json.RawMessage) (res toolapi.Result, panicValue any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			panicValue = recovered
			err = fmt.Errorf("tool invocation panicked: %v", recovered)
		}
	}()
	res, err = tool.Invoke(ctx, args)
	return
}
