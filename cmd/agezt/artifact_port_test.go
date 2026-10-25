// SPDX-License-Identifier: MIT

package main

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/modules/artifacts/artifact"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// withArtifacts opens the artifact store and index the composition root
// would open (W3.43) and hands the kernel the contract's ports.
//
// The artifact store and its metadata index are the DAEMON's files: the
// kernel holds the contract's ports. A kernel opened without them has no
// artifact store at all — the loop cannot offload an oversized output, the
// image ingress cannot persist an inbound image, and the tools cannot list
// artifacts.
func withArtifacts(t *testing.T, cfg kernelruntime.Config) kernelruntime.Config {
	t.Helper()
	store, err := artifact.Open(filepath.Join(cfg.BaseDir, "artifacts"))
	if err != nil {
		t.Fatalf("open artifact store: %v", err)
	}
	index, err := artifact.OpenIndex(store, filepath.Join(cfg.BaseDir, "artifacts"))
	if err != nil {
		t.Fatalf("open artifact index: %v", err)
	}
	cfg.Artifacts = store
	cfg.ArtifactIndex = index
	return cfg
}
