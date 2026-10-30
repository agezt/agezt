// SPDX-License-Identifier: MIT

package runtime_test

// The daemon's proof verb: the NAME it keeps and the BODY it gave away.
//
// The daemon used to hold the whole judgment — the evidence to gather, the
// criterion prompt, the reply to parse, the verdict to publish. It now holds
// the board's face and asks the module. These tests pin that with a fake
// board: if the daemon grew the judgment back, the fake would record nothing
// and the daemon's answer would be its own.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
	"github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// recordingProofBoard is a fake work module: it records what the daemon asks and
// answers with a task of its own.
//
// The embedded interface is nil on purpose: a verb the daemon must not reach
// for panics instead of silently answering.
type recordingProofBoard struct {
	workapi.BoardService
	proves []string
}

func (b *recordingProofBoard) Prove(ctx context.Context, corr, id, answer string) (workboardapi.Task, error) {
	b.proves = append(b.proves, corr+"|"+id+"|"+answer)
	return workboardapi.Task{ID: id, Title: "from the module"}, nil
}

func openBoardDaemon(t *testing.T) *runtime.Kernel {
	t.Helper()
	store, err := workboard.OpenStore(filepath.Join(t.TempDir(), "workboard"))
	if err != nil {
		t.Fatalf("open the workboard store: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Workboard: store})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// The daemon asks the MODULE to prove a task.
//
// The daemon's verb is a forwarder: the judgment lives in the work module,
// and the daemon's answer is the module's answer. A daemon that spelled the
// judgment out again would answer on its own and the fake would record
// nothing.
func TestTheDaemonAsksTheModuleToProveATask(t *testing.T) {
	k := openBoardDaemon(t)
	board := &recordingProofBoard{}
	k.SetWorkboard(board)

	task, err := k.ProveTask(context.Background(), "corr-1", "task-1", "the answer")
	if err != nil {
		t.Fatalf("ProveTask: %v", err)
	}
	if len(board.proves) != 1 {
		t.Fatalf("the module was asked %d times, want 1", len(board.proves))
	}
	if board.proves[0] != "corr-1|task-1|the answer" {
		t.Errorf("the module was asked %q, want the daemon's own arguments", board.proves[0])
	}
	if task.Title != "from the module" {
		t.Errorf("the answer came from %q, want the module's", task.Title)
	}
}

// The daemon refuses to prove when the module is not wired.
//
// The board is injected by the composition root. Until it is, the daemon has
// no board — and it says so with the CONTRACT's row, because the host may not
// name the module and the module may not name the host.
func TestTheDaemonRefusesToProveWhenTheModuleIsNotWired(t *testing.T) {
	k := openBoardDaemon(t)
	if _, err := k.ProveTask(context.Background(), "corr", "task-1", "an answer"); err != workboardapi.ErrNoStore {
		t.Errorf("ProveTask without the module: err = %v, want the contract's refusal", err)
	}
}

// The daemon's proof ports are the HOST's machinery, handed over as ways to
// ASK.
//
// The judge, the artifacts and the journal are the host's; the module never
// reaches into them. The daemon hands them over through the board's
// dependencies, and the proof asks through them.
func TestTheDaemonHandsTheProofPortsOver(t *testing.T) {
	k := openBoardDaemon(t)
	deps := k.WorkboardDependencies()
	if deps.Complete == nil {
		t.Error("the daemon must hand the judge over")
	}
	if deps.Model == nil {
		t.Error("the daemon must hand the model over")
	}
	if deps.VerifyCompletion == nil {
		t.Error("the daemon must hand the plain completion judge over")
	}
	if deps.Publish == nil {
		t.Error("the daemon must hand the bus over")
	}
	if deps.TaskProved == nil {
		t.Error("the daemon must tell the module when a task was proved")
	}
}

// The daemon hands its journal over when it has one.
//
// The journal is the daemon's own (Open always opens one), and the module
// walks it through the port — it never reaches the daemon's journal package.
// A daemon with no journal hands over no journal at all: the port is nil, and
// the proof records no journal range rather than walking a journal that is
// not there.
func TestTheDaemonHandsItsJournalOver(t *testing.T) {
	k := openBoardDaemon(t)
	deps := k.WorkboardDependencies()
	if deps.Journal == nil {
		t.Error("the daemon must hand the journal it keeps over")
	}
	if deps.Artifacts != nil {
		t.Error("the daemon must not hand an artifact index it does not have")
	}
}
