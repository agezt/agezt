// SPDX-License-Identifier: MIT

package files

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// RunnerFromHandle returns the daemon's tool runner from the handle.
//
// The handle is not named by the caller: the files family fills its own
// runner from it. A handle that is not the daemon's own yields a runner that
// REFUSES — a file operation is answered by nobody rather than by a runner
// that does not exist.
func RunnerFromHandle(h any) Runner {
	k, ok := h.(*runtime.Kernel)
	if !ok {
		return refusingRunner{}
	}
	return k
}

// refusingRunner is the runner a host without a daemon gets.
type refusingRunner struct{}

func (refusingRunner) RunToolWithLookup(context.Context, string, string, string, json.RawMessage, toolapi.ToolLookup) (toolapi.Result, error) {
	return toolapi.Result{}, errors.New("the tool runner is not available")
}
