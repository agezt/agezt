// SPDX-License-Identifier: MIT

package workboard_test

import (
	"context"
	"strings"
	"testing"

	appwork "github.com/agezt/agezt/kernel/app/workboard"
)

// TestTheServiceRefusesWithoutABoard: the board used to be opened by the
// kernel, so the app layer's port was never nil and the app never had to say
// what happens without one. Now the store file is the host's (W3.47) and a
// host may open a kernel without a board — and every app operation must
// REFUSE instead of dereferencing nil.
//
// The refusal is the same answer everywhere: "the task board is not available
// on this daemon".
func TestTheServiceRefusesWithoutABoard(t *testing.T) {
	service := appwork.New(nil)
	for _, c := range []struct {
		name string
		run  func() error
	}{
		{"list", func() error {
			_, err := service.List(context.Background(), appwork.ListInput{})
			return err
		}},
		{"lanes", func() error {
			_, err := service.Lanes(context.Background(), appwork.ListInput{})
			return err
		}},
		{"show", func() error {
			_, err := service.Show(context.Background(), appwork.ShowInput{ID: "id"})
			return err
		}},
	} {
		err := c.run()
		if err == nil {
			t.Errorf("%s: the operation reported success without a board", c.name)
			continue
		}
		if !strings.Contains(err.Error(), "not available") {
			t.Errorf("%s: the refusal = %v, want the board's plain refusal", c.name, err)
		}
	}
}

// TestTheWatchRefusesWithoutABoard: the watch surface holds the store too,
// and it must refuse the same way.
func TestTheWatchRefusesWithoutABoard(t *testing.T) {
	watch := appwork.NewWatch(nil, nil)
	_, err := watch.Watch(context.Background(), appwork.WatchInput{ID: "id"})
	if err == nil {
		t.Fatal("the watch reported success without a board")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("the refusal = %v, want the board's plain refusal", err)
	}
}

// TestTheDispatchRefusesWithoutABoard: the dispatch surface drives the store
// directly (it claims, blocks and reads dependencies), so it must refuse
// before it reaches for it.
func TestTheDispatchRefusesWithoutABoard(t *testing.T) {
	dispatch := appwork.NewDispatch(nil, nil, nil, nil)
	_, err := dispatch.Dispatch(context.Background(), appwork.DispatchInput{ID: "id"})
	if err == nil {
		t.Fatal("the dispatch reported success without a board")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("the refusal = %v, want the board's plain refusal", err)
	}
}

// TestTheExecutionRefusesWithoutABoard: the execution surface reads the task
// it is about to run, so it must refuse before it reads.
func TestTheExecutionRefusesWithoutABoard(t *testing.T) {
	execution := appwork.NewExecution(nil, nil, nil, nil, nil, nil)
	_, err := execution.Run(context.Background(), appwork.ExecutionInput{})
	if err == nil {
		t.Fatal("the execution reported success without a board")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("the refusal = %v, want the board's plain refusal", err)
	}
}
