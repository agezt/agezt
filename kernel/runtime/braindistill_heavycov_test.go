// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestDistillBrain_Halted covers the halted-guard branch of DistillBrain and
// DistillProfile: a halted kernel refuses both with ErrHalted.
func TestDistillBrain_Halted(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	k.Halt()

	if _, err := k.DistillBrain(context.Background(), "corr"); !errors.Is(err, runtime.ErrHalted) {
		t.Errorf("DistillBrain halted err = %v, want ErrHalted", err)
	}
	if _, err := k.DistillProfile(context.Background(), "corr"); !errors.Is(err, runtime.ErrHalted) {
		t.Errorf("DistillProfile halted err = %v, want ErrHalted", err)
	}
}
