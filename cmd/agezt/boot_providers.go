// SPDX-License-Identifier: MIT

package main

// Provider bootstrap glue (Phase 2.4): the selection/registration/reload logic
// itself lives in plugins/providerboot — this file only assembles the daemon's
// dependency bundle for it. awschain.go (the credential chain builders) stays
// in cmd/agezt because the AWS/IMDS stages are host concerns, not provider
// bootstrap.

import (
	"io"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/plugins/providerboot"
)

// providerDeps builds the providerboot dependency bundle from a catalog
// snapshot + the credentials vault, rebuilding the catalog-scoped chained
// credential lookup (vault → env → AWS chain) so a freshly rotated key or
// synced catalog is honoured. Get is left nil (os.Getenv — the Config Center
// injects operator edits into the process env before boot).
func providerDeps(cat *catalog.Catalog, credStore *creds.Store, baseDir string, stderr io.Writer) providerboot.Deps {
	lookup, _ := buildAWSCredChain(catalogScopedVaultLookup(cat, credStore.Lookup))
	return providerboot.Deps{Catalog: cat, Lookup: lookup, BaseDir: baseDir, Stderr: stderr}
}

// firstRunSetupNeeded reports whether the daemon-ready banner should carry the
// "setup needed" first-run nudge: true exactly when the boot resolved to the
// UNCONFIGURED sentinel primary (no provider selected → every LLM run refuses).
// Keyed off the PRIMARY name, never the model id — the old `model == "mock"`
// check fired exactly backwards: the sentinel resolves model "" (no nudge), and
// "mock" only appears via the deliberate AGEZT_DEMO_ECHO=1 escape hatch (nudge
// on a correctly configured demo daemon).
func firstRunSetupNeeded(res *providerboot.Result) bool {
	return res != nil && res.Primary == providerboot.UnconfiguredName
}
