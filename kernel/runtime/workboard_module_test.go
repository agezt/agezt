// SPDX-License-Identifier: MIT

package runtime_test

// The daemon's board: the NAMES it keeps and the BODY it gave away.
//
// The daemon used to hold the store and spell out the lifecycle itself. It
// now holds the WORK MODULE's face and asks it. These tests pin that with a
// fake module: if the daemon grew the lifecycle back, the fake would record
// nothing and the daemon's answer would be its own.

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
	"github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// recordingBoard is a fake work module: it records what the daemon asks and
// answers with a task of its own.
//
// The embedded interface is nil on purpose: a verb the daemon must not reach
// for panics instead of silently answering.
type recordingBoard struct {
	workapi.Service
	creates   []string
	claims    []string
	completes []string
}

func (b *recordingBoard) Create(corr string, spec workboardapi.CreateSpec) (workboardapi.Task, bool, error) {
	b.creates = append(b.creates, corr+"|"+spec.Title)
	return workboardapi.Task{ID: "task-0", Title: "from the module"}, true, nil
}

func (b *recordingBoard) Claim(corr, id, agent, runID string) (workboardapi.Task, error) {
	b.claims = append(b.claims, corr+"|"+id+"|"+agent+"|"+runID)
	return workboardapi.Task{ID: id, Title: "from the module"}, nil
}

func (b *recordingBoard) Complete(corr, id, actor string) (workboardapi.Task, error) {
	b.completes = append(b.completes, corr+"|"+id+"|"+actor)
	return workboardapi.Task{ID: id, Title: "from the module"}, nil
}

func openDaemon(t *testing.T, withStore bool) *runtime.Kernel {
	t.Helper()
	cfg := runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}
	if withStore {
		store, err := workboard.OpenStore(filepath.Join(cfg.BaseDir, "workboard"))
		if err != nil {
			t.Fatalf("open the workboard store: %v", err)
		}
		cfg.Workboard = store
	}
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// The daemon asks the MODULE for the board.
//
// The daemon's verbs are forwarders: the lifecycle lives in the work module,
// and the daemon's answer is the module's answer. A daemon that spelled the
// lifecycle out again would answer on its own and the fake would record
// nothing.
func TestTheDaemonAsksTheModuleForTheBoard(t *testing.T) {
	k := openDaemon(t, true)
	board := &recordingBoard{}
	k.SetWorkboard(board)

	task, err := k.ClaimWorkboardTask("corr-1", "task-1", "agent-1", "run-1")
	if err != nil {
		t.Fatalf("ClaimWorkboardTask: %v", err)
	}
	if len(board.claims) != 1 {
		t.Fatalf("the module was asked %d times, want 1", len(board.claims))
	}
	if board.claims[0] != "corr-1|task-1|agent-1|run-1" {
		t.Errorf("the module was asked %q, want the daemon's own arguments", board.claims[0])
	}
	if task.Title != "from the module" {
		t.Errorf("the answer came from %q, want the module's", task.Title)
	}

	if _, _, err := k.CreateWorkboardTask("corr-0", workboardapi.CreateSpec{Title: "fresh"}); err != nil {
		t.Fatalf("CreateWorkboardTask: %v", err)
	}
	if len(board.creates) != 1 || board.creates[0] != "corr-0|fresh" {
		t.Errorf("the module was asked to create %v, want the daemon's own arguments", board.creates)
	}

	if _, err := k.CompleteWorkboardTask("corr-2", "task-2", "actor"); err != nil {
		t.Fatalf("CompleteWorkboardTask: %v", err)
	}
	if len(board.completes) != 1 || board.completes[0] != "corr-2|task-2|actor" {
		t.Errorf("the module was asked to complete %v, want the daemon's own arguments", board.completes)
	}
}

// The daemon refuses the board when the module is not wired.
//
// The module is injected by the composition root. Until it is, the daemon
// has no board — and it says so with the CONTRACT's row, because the host may
// not name the module and the module may not name the host.
func TestTheDaemonRefusesTheBoardWhenTheModuleIsNotWired(t *testing.T) {
	k := openDaemon(t, true)

	if _, _, err := k.CreateWorkboardTask("corr", workboardapi.CreateSpec{Title: "none"}); err != workboardapi.ErrNoStore {
		t.Errorf("Create without the module: err = %v, want the contract's refusal", err)
	}
	if _, err := k.ClaimWorkboardTask("corr", "id", "agent", "run"); err != workboardapi.ErrNoStore {
		t.Errorf("Claim without the module: err = %v, want the contract's refusal", err)
	}
	if _, err := k.CompleteWorkboardTask("corr", "id", "actor"); err != workboardapi.ErrNoStore {
		t.Errorf("Complete without the module: err = %v, want the contract's refusal", err)
	}
	if _, err := k.SweepStaleWorkboardClaims("corr", "actor", time.Minute, 10); err != workboardapi.ErrNoStore {
		t.Errorf("SweepStaleClaims without the module: err = %v, want the contract's refusal", err)
	}
}

// The daemon still holds the HOST's board file.
//
// The store file is the host's business (W3.47): the daemon opens it and
// hands the port to the module. The module's service is the lifecycle, and
// the port is still the daemon's to answer for — the plugins' contract speaks
// the store's port.
func TestTheDaemonStillHoldsTheHostsBoardFile(t *testing.T) {
	k := openDaemon(t, true)
	if k.Workboard() == nil {
		t.Fatal("the daemon must hold the board store it was given")
	}
	k.SetWorkboard(workboard.NewService(k.Workboard(), k.WorkboardDependencies()))
	if k.Workboard() == nil {
		t.Fatal("wiring the module must not take the board file away from the daemon")
	}
}
