// SPDX-License-Identifier: MIT

package runtime_test

// The daemon's board: the FILE it keeps and the FACE it holds.
//
// The board's operations are the WORK MODULE's. The daemon used to hold
// SIXTEEN forwarders over them — a second spelling of every board verb — and
// the application layer and the plugin called the daemon for them.
//
// Now the MODULE implements the CONTRACT's runner port over its own service
// (kernel/modules/work/workboard/runner.go), and the daemon holds two things:
// the board's FILE (the store it opened, W3.47) and the module's FACE (the
// runner the composition root hands over).
//
// These tests pin that with a fake module: the runner's answers come from the
// module, and the daemon spells nothing out.

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
	"github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// recordingBoard is a fake work module: it records what it is asked and
// answers with a task of its own.
//
// The embedded interface is nil on purpose: a verb the module must not reach
// for panics instead of silently answering.
type recordingBoard struct {
	workapi.BoardService
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

// The daemon holds the board's FILE and the module's FACE.
//
// The face answers in the CONTRACT's names — the board's users say them that
// way — and the answer comes from the module's service. A daemon that spelled
// the lifecycle out again would answer on its own and the fake would record
// nothing.
func TestTheDaemonHoldsTheBoardsFileAndTheModulesFace(t *testing.T) {
	k := openDaemon(t, true)
	board := &recordingBoard{}
	k.SetBoardRunner(workboard.NewRunner(board))

	if k.Workboard() == nil {
		t.Fatal("the daemon must hold the board store it was given")
	}
	runner := k.BoardRunner()
	if runner == nil {
		t.Fatal("the daemon must hold the board's runner it was given")
	}

	task, err := runner.ClaimWorkboardTask("corr-1", "task-1", "agent-1", "run-1")
	if err != nil {
		t.Fatalf("ClaimWorkboardTask: %v", err)
	}
	if len(board.claims) != 1 {
		t.Fatalf("the module was asked %d times, want 1", len(board.claims))
	}
	if board.claims[0] != "corr-1|task-1|agent-1|run-1" {
		t.Errorf("the module was asked %q, want the caller's own arguments", board.claims[0])
	}
	if task.Title != "from the module" {
		t.Errorf("the answer came from %q, want the module's", task.Title)
	}

	if _, _, err := runner.CreateWorkboardTask("corr-0", workboardapi.CreateSpec{Title: "fresh"}); err != nil {
		t.Fatalf("CreateWorkboardTask: %v", err)
	}
	if len(board.creates) != 1 || board.creates[0] != "corr-0|fresh" {
		t.Errorf("the module was asked to create %v, want the caller's own arguments", board.creates)
	}

	if _, err := runner.CompleteWorkboardTask("corr-2", "task-2", "actor"); err != nil {
		t.Fatalf("CompleteWorkboardTask: %v", err)
	}
	if len(board.completes) != 1 || board.completes[0] != "corr-2|task-2|actor" {
		t.Errorf("the module was asked to complete %v, want the caller's own arguments", board.completes)
	}
}

// The daemon holds NO face when the module is not wired.
//
// The runner is injected by the composition root. Until it is, the daemon has
// no board to hand over — and the refusal lives where the lifecycle lives
// (the module's service, with the CONTRACT's row), because the host may not
// name the module and the module may not name the host.
func TestTheDaemonHoldsNoBoardFaceWhenTheModuleIsNotWired(t *testing.T) {
	k := openDaemon(t, true)

	if got := k.BoardRunner(); got != nil {
		t.Fatalf("a kernel with no board runner must yield a nil face, got %v", got)
	}
	if _, _, err := workboard.NewService(nil, workapi.BoardDependencies{}).Create("corr", workboardapi.CreateSpec{Title: "none"}); err != workboardapi.ErrNoStore {
		t.Errorf("Create without a store: err = %v, want the contract's refusal", err)
	}
}

// The daemon still holds the HOST's board file.
//
// The store file is the host's business (W3.47): the daemon opens it and
// hands the port to the module. The module's service is the lifecycle, and the
// port is still the daemon's to answer for — the plugins' contract speaks the
// store's port.
func TestTheDaemonStillHoldsTheHostsBoardFile(t *testing.T) {
	k := openDaemon(t, true)
	if k.Workboard() == nil {
		t.Fatal("the daemon must hold the board store it was given")
	}
	k.SetBoardRunner(workboard.NewRunner(workboard.NewService(k.Workboard(), k.WorkboardDependencies())))
	if k.Workboard() == nil {
		t.Fatal("wiring the module must not take the board file away from the daemon")
	}
}
