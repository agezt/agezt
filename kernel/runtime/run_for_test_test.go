// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"

	"github.com/agezt/agezt/kernel/runtime"
)

// runForTest runs one run the way the retired daemon door used to: it mints
// the correlation id at the HOST's identity door (NewCorrelation) and hands
// that id to the runs module's runner face (W3.76ac).
//
// The daemon no longer owns a `Run` door of its own — the run engine answers
// `RunWith` and the host owns the identity. Tests that need both the answer
// and the correlation keep the old three-value shape through this helper.
func runForTest(k *runtime.Kernel, ctx context.Context, intent string) (string, string, error) {
	corr := k.NewCorrelation()
	ans, err := k.RunnerService().RunWith(ctx, corr, intent)
	return ans, corr, err
}
