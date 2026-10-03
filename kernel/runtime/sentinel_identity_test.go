// SPDX-License-Identifier: MIT

package runtime_test

import (
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/runtime/runexec"
)

// The run-engine sentinels were defined twice — one errors.New in runtime and
// one in runexec, same text, different identity — so errors.Is told them
// apart and each crossing needed a hand-written translation. They are now one
// value re-exported under both names.
func TestRunSentinels_OneIdentity(t *testing.T) {
	for _, c := range []struct {
		name    string
		rt, rex error
	}{
		{"ErrHalted", runtime.ErrHalted, runexec.ErrHalted},
		{"ErrNoVisionModel", runtime.ErrNoVisionModel, runexec.ErrNoVisionModel},
	} {
		if !errors.Is(c.rt, c.rex) || !errors.Is(c.rex, c.rt) {
			t.Errorf("%s: runtime and runexec sentinels are different identities", c.name)
		}
	}
}
