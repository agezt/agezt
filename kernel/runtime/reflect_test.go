// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestReflectThroughKernel(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	// A run produces task.received/completed for the fold to observe.
	if _, _, err := k.Run(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}

	rep, err := k.Reflect().Reflect(context.Background(), "refl-1")
	if err != nil {
		t.Fatalf("reflect: %v", err)
	}
	if rep.Observations.TasksStarted < 1 || rep.Observations.TasksCompleted < 1 {
		t.Errorf("reflection should observe the run: %+v", rep.Observations)
	}
	if countKind(t, k, event.KindReflectionCompleted) != 1 {
		t.Error("reflect must journal reflection.completed")
	}

	// Latest reads it back.
	if _, ok := k.Reflect().Latest(); !ok {
		t.Error("Latest should return the just-written report")
	}
}
