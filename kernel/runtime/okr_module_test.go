// SPDX-License-Identifier: MIT

package runtime_test

// The daemon's objectives: the NAMES it keeps and the BODY it gave away.
//
// The daemon used to hold the library and spell out the objective lifecycle
// itself. It now holds the WORK MODULE's face and asks it. These tests pin
// that with a fake module: if the daemon grew the lifecycle back, the fake
// would record nothing and the daemon's answer would be its own.

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
	"github.com/agezt/agezt/kernel/modules/work/okr"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// recordingLibrary is a fake work module: it records what the daemon asks and
// answers with a goal of its own.
//
// The embedded interface is nil on purpose: a verb the daemon must not reach
// for panics instead of silently answering.
type recordingLibrary struct {
	workapi.ObjectiveService
	creates  []string
	links    []string
	archives []string
	rolls    []string
}

func (l *recordingLibrary) Create(corr string, spec okrapi.CreateSpec) (okrapi.Objective, error) {
	l.creates = append(l.creates, corr+"|"+spec.Title)
	return okrapi.Objective{ID: "obj-0", Title: "from the module"}, nil
}

func (l *recordingLibrary) LinkTask(corr, objID, krID, taskID string) (okrapi.Objective, error) {
	l.links = append(l.links, corr+"|"+objID+"|"+krID+"|"+taskID)
	return okrapi.Objective{ID: objID, Title: "from the module"}, nil
}

func (l *recordingLibrary) Archive(corr, objID string) (okrapi.Objective, error) {
	l.archives = append(l.archives, corr+"|"+objID)
	return okrapi.Objective{ID: objID, Title: "from the module"}, nil
}

// Progress answers with the module's OWN rollup — the daemon must not roll a
// goal up on its own.
func (l *recordingLibrary) Progress(o okrapi.Objective) okrapi.ObjectiveProgress {
	l.rolls = append(l.rolls, o.ID)
	return okrapi.ObjectiveProgress{ObjectiveID: o.ID, Percent: 100, Achieved: true}
}

// ProgressOf answers with the module's OWN goal and rollup.
func (l *recordingLibrary) ProgressOf(id string) (okrapi.Objective, okrapi.ObjectiveProgress, bool) {
	l.rolls = append(l.rolls, id)
	return okrapi.Objective{ID: id, Title: "from the module"}, okrapi.ObjectiveProgress{ObjectiveID: id, Percent: 100, Achieved: true}, true
}

func openObjectiveDaemon(t *testing.T, withStore bool) *runtime.Kernel {
	t.Helper()
	cfg := runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}
	if withStore {
		store, err := okr.OpenStore(filepath.Join(cfg.BaseDir, "okr"))
		if err != nil {
			t.Fatalf("open the okr store: %v", err)
		}
		cfg.OKRStore = store
	}
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// The daemon asks the MODULE for the objectives.
//
// The daemon's verbs are forwarders: the lifecycle lives in the work module,
// and the daemon's answer is the module's answer. A daemon that spelled the
// lifecycle out again would answer on its own and the fake would record
// nothing.
func TestTheDaemonAsksTheModuleForTheObjectives(t *testing.T) {
	k := openObjectiveDaemon(t, true)
	library := &recordingLibrary{}
	k.SetObjectiveLibrary(library)

	goal, err := k.CreateObjective("corr-1", okrapi.CreateSpec{Title: "owned"})
	if err != nil {
		t.Fatalf("CreateObjective: %v", err)
	}
	if len(library.creates) != 1 {
		t.Fatalf("the module was asked %d times, want 1", len(library.creates))
	}
	if library.creates[0] != "corr-1|owned" {
		t.Errorf("the module was asked %q, want the daemon's own arguments", library.creates[0])
	}
	if goal.Title != "from the module" {
		t.Errorf("the answer came from %q, want the module's", goal.Title)
	}

	if _, err := k.LinkObjectiveTask("corr-2", "obj-1", "kr-1", "task-1"); err != nil {
		t.Fatalf("LinkObjectiveTask: %v", err)
	}
	if len(library.links) != 1 || library.links[0] != "corr-2|obj-1|kr-1|task-1" {
		t.Errorf("the module was asked to link %v, want the daemon's own arguments", library.links)
	}

	if _, err := k.ArchiveObjective("corr-3", "obj-1"); err != nil {
		t.Fatalf("ArchiveObjective: %v", err)
	}
	if len(library.archives) != 1 || library.archives[0] != "corr-3|obj-1" {
		t.Errorf("the module was asked to archive %v, want the daemon's own arguments", library.archives)
	}
}

// The daemon refuses the objectives when the module is not wired.
//
// The module is injected by the composition root. Until it is, the daemon has
// no library — and it says so with the CONTRACT's row, because the host may
// not name the module and the module may not name the host.
func TestTheDaemonRefusesTheObjectivesWhenTheModuleIsNotWired(t *testing.T) {
	k := openObjectiveDaemon(t, true)

	if _, err := k.CreateObjective("corr", okrapi.CreateSpec{Title: "none"}); err != okrapi.ErrNoLibrary {
		t.Errorf("Create without the module: err = %v, want the contract's refusal", err)
	}
	if _, err := k.AddObjectiveKeyResult("corr", "o-1", "kr", 1); err != okrapi.ErrNoLibrary {
		t.Errorf("AddKeyResult without the module: err = %v, want the contract's refusal", err)
	}
	if _, err := k.LinkObjectiveTask("corr", "o-1", "kr-1", "t-1"); err != okrapi.ErrNoLibrary {
		t.Errorf("Link without the module: err = %v, want the contract's refusal", err)
	}
	if _, err := k.UnlinkObjectiveTask("corr", "o-1", "kr-1", "t-1"); err != okrapi.ErrNoLibrary {
		t.Errorf("Unlink without the module: err = %v, want the contract's refusal", err)
	}
	if _, err := k.ArchiveObjective("corr", "o-1"); err != okrapi.ErrNoLibrary {
		t.Errorf("Archive without the module: err = %v, want the contract's refusal", err)
	}
}

// The daemon still holds the HOST's library file.
//
// The store file is the host's business (W3.53): the daemon opens it and hands
// the port to the module. The module's service is the lifecycle, and the port
// is still the daemon's to answer for.
func TestTheDaemonStillHoldsTheHostsLibraryFile(t *testing.T) {
	k := openObjectiveDaemon(t, true)
	if k.OKR() == nil {
		t.Fatal("the daemon must hold the library it was given")
	}
	k.SetObjectiveLibrary(okr.NewService(k.OKR(), k.ObjectiveLibraryDependencies()))
	if k.OKR() == nil {
		t.Fatal("wiring the module must not take the library file away from the daemon")
	}
}

// The daemon's rollup comes from the MODULE.
//
// The rollup rule — what counts toward a key result — belongs to the library
// that keeps the goals. A daemon that rolled up on its own would answer with
// its own rule, and the two would drift apart.
func TestTheDaemonRollsUpThroughTheModule(t *testing.T) {
	k := openObjectiveDaemon(t, true)
	library := &recordingLibrary{}
	k.SetObjectiveLibrary(library)

	goal := okrapi.Objective{ID: "obj-1", KeyResults: []okrapi.KeyResult{{ID: "kr-1", Target: 1, TaskIDs: []string{"t-1"}}}}
	progress := k.Rollup(goal)
	if len(library.rolls) != 1 {
		t.Fatalf("the module was asked to roll up %d times, want 1", len(library.rolls))
	}
	if library.rolls[0] != "obj-1" {
		t.Errorf("the module was asked about %q, want the daemon's own goal", library.rolls[0])
	}
	if !progress.Achieved || progress.Percent != 100 {
		t.Errorf("the rollup = %+v, want the module's own answer", progress)
	}

	got, pr, ok := k.ObjectiveRollup("obj-1")
	if !ok || got.Title != "from the module" {
		t.Fatalf("ObjectiveRollup = %+v %v, want the module's goal", got, ok)
	}
	if !pr.Achieved {
		t.Errorf("the rollup = %+v, want the module's own answer", pr)
	}
}

// Without the module, the daemon's rollup counts nothing.
//
// The same answer it gave before the module was wired: no module, no board,
// nothing counts.
func TestTheDaemonCountsNothingWithoutTheModule(t *testing.T) {
	k := openObjectiveDaemon(t, true)

	goal := okrapi.Objective{ID: "obj-1", KeyResults: []okrapi.KeyResult{{ID: "kr-1", Target: 1, TaskIDs: []string{"t-1"}}}}
	if progress := k.Rollup(goal); progress.Achieved {
		t.Error("without the module nothing counts toward a goal")
	}
	if _, _, ok := k.ObjectiveRollup("obj-1"); ok {
		t.Error("without the module there is no goal to roll up")
	}
}
