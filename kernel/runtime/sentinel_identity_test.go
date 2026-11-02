// SPDX-License-Identifier: MIT

package runtime_test

import (
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/modules/runs/runner"
	"github.com/agezt/agezt/kernel/runtime"
)

// The run-engine sentinels were defined twice — one errors.New in runtime and
// one in runner, same text, different identity — so errors.Is told them
// apart and each crossing needed a hand-written translation. They are now one
// value re-exported under both names.
func TestRunSentinels_OneIdentity(t *testing.T) {
	for _, c := range []struct {
		name    string
		rt, rex error
	}{
		{"ErrHalted", runtime.ErrHalted, runner.ErrHalted},
		{"ErrNoVisionModel", runtime.ErrNoVisionModel, runner.ErrNoVisionModel},
	} {
		if !errors.Is(c.rt, c.rex) || !errors.Is(c.rex, c.rt) {
			t.Errorf("%s: runtime and runner sentinels are different identities", c.name)
		}
	}
}
