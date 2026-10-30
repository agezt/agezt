// SPDX-License-Identifier: MIT

package files

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// RunnerFromKernel returns the daemon's tool runner from the daemon's own
// kernel.
//
// The runner IS the daemon's own kernel: the files family fills its own
// runner from it, and the caller never names the kernel's verbs. A daemon
// that does not exist yields a runner that REFUSES — a file operation is
// answered by nobody rather than by a runner that does not exist.
func RunnerFromKernel(k *runtime.Kernel) Runner {
	if k == nil {
		return refusingRunner{}
	}
	return k
}

// refusingRunner is the runner a host without a daemon gets.
type refusingRunner struct{}

func (refusingRunner) RunToolWithLookup(context.Context, string, string, string, json.RawMessage, toolapi.ToolLookup) (toolapi.Result, error) {
	return toolapi.Result{}, errors.New("the tool runner is not available")
}
