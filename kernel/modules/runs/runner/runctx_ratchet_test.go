// SPDX-License-Identifier: MIT

package runner

// The ratchet of W3.76l: the run engine reads the RUN'S OWN CONTEXT from the
// module that owns it.
//
// The run context's vocabulary — the wake's fields, the images, the json
// mode, the cost ceiling, the model chain, the agent's daily ceiling, the
// actor — belongs to kernel/modules/runs/runctx. The run engine LIVES in the
// runs module, so asking the HOST for those values was a hop that bought
// nothing: the host kept fifteen forwarding copies of the module's own names
// (the daemon's `WakeContextSource`, `MaxCostFromCtx`, `WithActorCorrelation`
// and their siblings), and the engine went through them to read the run's own
// decoration.
//
// What stays on the port is the HOST's own state: the effective config (the
// model the run falls back to, the heuristic bypass), the resume ticket, the
// seed and the checkpoint callback. Those the host owns and the module
// cannot answer.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting one of the module's names back
// behind the port has to make this file fail.

import (
	"os"
	"strings"
	"testing"
)

// TestTheRunnerReadsTheRunContextFromTheModule pins the direction: the run
// engine asks the module for the run's context.
//
// The check is over the runner's PRODUCTION files — the test files may
// legitimately mention a name.
func TestTheRunnerReadsTheRunContextFromTheModule(t *testing.T) {
	for _, want := range []string{
		"runctx.WithActor(",
		"runctx.SystemAgentFromCtx(",
		"runctx.AgentDailyMcFromCtx(",
		"runctx.ModelChainFromCtx(",
		"runctx.WakeContextFromCtx(",
		"runctx.ImagesFromCtx(",
		"runctx.JSONModeFromCtx(",
		"runctx.MaxCostFromCtx(",
	} {
		if !runnerPackageContains(t, want) {
			t.Errorf("the run engine must read the run's context through %s", want)
		}
	}
}

// TestThePortCarriesNoRunContextVocabulary pins the other half: the host's
// port does not promise the run context's vocabulary any more. A port entry
// is a PROMISE the module may rely on, and this one was never kept.
func TestThePortCarriesNoRunContextVocabulary(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	text := string(src)
	for _, gone := range []string{
		"SystemAgentFromCtx(ctx context.Context)",
		"AgentDailyMcFromCtx(ctx context.Context)",
		"ModelChainFromCtx(ctx context.Context)",
		"WakeContextSource(ctx context.Context)",
		"WakeContextReason(ctx context.Context)",
		"WakeContextScheduleID(ctx context.Context)",
		"WakeContextStandingID(ctx context.Context)",
		"WakeContextStandingName(ctx context.Context)",
		"WakeContextTriggerSubject(ctx context.Context)",
		"WakeContextParentCorrelation(ctx context.Context)",
		"ImagesFromCtx(ctx context.Context)",
		"JSONModeFromCtx(ctx context.Context)",
		"MaxCostFromCtx(ctx context.Context)",
		"WithActorCorrelation(ctx context.Context",
		"ActorFromCtx(ctx context.Context)",
	} {
		if strings.Contains(text, gone) {
			t.Errorf("the host's port still promises %s — the run context's vocabulary is the module's", gone)
		}
	}
}

// TestThePortStillCarriesTheHostsOwnState pins what the port is FOR: the
// host's own answers, which the module cannot give.
func TestThePortStillCarriesTheHostsOwnState(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	text := string(src)
	for _, keep := range []string{
		"ResumeOwnedKindFromCtx(ctx context.Context)",
		"ResumeSeedFromCtx(ctx context.Context)",
		"ResumeCheckpointFn(corr string)",
		"ResolveRunModel(ctx context.Context)",
		"DisableHeuristicBypass(ctx context.Context)",
	} {
		if !strings.Contains(text, keep) {
			t.Errorf("the host's port lost %s — the host owns that state", keep)
		}
	}
}

// runnerPackageContains reports whether the runner's own production sources
// hold the given text.
func runnerPackageContains(t *testing.T, needle string) bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the runner package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if strings.Contains(string(src), needle) {
			return true
		}
	}
	return false
}
