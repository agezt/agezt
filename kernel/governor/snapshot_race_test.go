// SPDX-License-Identifier: MIT

package governor_test

// Regression test for F1 (Snapshot/map race). Snapshot() used to release
// g.mu before reading g.spentByTaskToday at line 200 of governor_usage.go,
// racing the recordUsage write at line 60. The runtime's built-in map race
// detector aborts a program that hits this concurrently with Complete;
// -race trips immediately.
//
// The test must complete without a panic AND without a -race data-race
// report when run as `go test -race`.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/governor"
)

func TestSnapshot_NoConcurrentMapRaceWithRecordUsage(t *testing.T) {
	r := governor.NewRegistry()
	prov := &fakeProvider{name: "p", resp: okResp("mock", 1, 1)}
	mustRegister(t, r, &governor.ProviderInfo{
		Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey,
	})
	g, err := governor.New(governor.Config{
		Registry:    r,
		TaskBudgets: map[string]int64{"demo": 1_000_000_000},
		Now:         func() time.Time { return time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	stop := make(chan struct{})
	var writes, reads int64
	var wg sync.WaitGroup
	wg.Add(2)

	// Writer: drives Complete, which on success calls recordUsage and
	// writes g.spentByTaskToday["demo"] under g.mu.
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			_, _ = g.Complete(ctx, agent.CompletionRequest{
				Model:        "mock",
				TaskType:     "demo",
				CorrelationID: "race",
			})
			cancel()
			atomic.AddInt64(&writes, 1)
		}
	}()

	// Reader: drives Snapshot, which reads g.spentByTaskToday at line 200
	// (this is the line the runtime panic points to when unfixed).
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			snap := g.Snapshot()
			// Touch PerTask to ensure the loop ran.
			_ = snap.PerTask
			atomic.AddInt64(&reads, 1)
		}
	}()

	// Run hard enough that any concurrent map access fires. Run with
	// `go test -race` to detect without the runtime crash; without
	// -race, the runtime panic itself is the failure signal.
	deadline := time.NewTimer(750 * time.Millisecond)
	defer deadline.Stop()
	<-deadline.C
	close(stop)
	wg.Wait()

	if writes < 1 || reads < 1 {
		t.Fatalf("writers=%d readers=%d: expected both >0 (test did not exercise the race window)",
			writes, reads)
	}

	// Sanity: a final Snapshot still returns a well-formed BudgetSnapshot
	// and the per-task row for "demo" is present (the contract Snapshot
	// promises even when nothing has been spent on that task).
	snap := g.Snapshot()
	found := false
	for _, row := range snap.PerTask {
		if row.TaskType == "demo" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Snapshot.PerTask missing the 'demo' row that the config registered")
	}
}
