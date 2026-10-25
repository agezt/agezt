// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/modules/work/workboard"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// withWorkboard opens the task board the composition root would open (W3.47)
// and hands the kernel the contract's host port.
//
// The board's file is the DAEMON's: the kernel holds the port and drives the
// lifecycle through it. A kernel opened without one has no board at all — the
// board operations refuse instead of calling into nothing.
func withWorkboard(t *testing.T, cfg kernelruntime.Config) kernelruntime.Config {
	t.Helper()
	store, err := workboard.OpenStore(filepath.Join(cfg.BaseDir, "workboard"))
	if err != nil {
		t.Fatalf("open the workboard store: %v", err)
	}
	cfg.Workboard = store
	return cfg
}

// TestOpenAppKernelWiresTheTaskBoard: the task board is the DAEMON's file
// (W3.47) — the kernel holds the contract's host port and the composition
// root opens the store.
//
// The daemon end: the port is live, and it is the LIVE store — a task created
// through the kernel has to land in the file the daemon opened, because that
// file is what the next start reads.
func TestOpenAppKernelWiresTheTaskBoard(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if k.Workboard() == nil {
		t.Fatal("the daemon did not wire the task board — the kernel holds no port")
	}
	task, created, err := k.CreateWorkboardTask("corr", workboardapi.CreateSpec{Title: "wired"})
	if err != nil {
		t.Fatalf("CreateWorkboardTask: %v", err)
	}
	if !created || task.Title != "wired" {
		t.Fatalf("the board did not take the task: %+v", task)
	}

	// The store is a FILE, and the daemon owns it: the task has to be on disk,
	// under the daemon's base directory.
	if _, err := os.Stat(filepath.Join(dir, "workboard", "workboard.json")); err != nil {
		t.Fatalf("the daemon did not open the board's own file: %v", err)
	}
}
