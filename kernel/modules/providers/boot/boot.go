// SPDX-License-Identifier: MIT

package boot

// Provenance: Package boot owns provider bootstrap for the daemon: primary
//             selection, alternate registration, the governor's construction, and
//             the hot-reload path. It is the providers module's engine: the
//             wire adapters it drives (the compat family, the offline mock, the
//             ChatGPT Responses backend) live above the module and arrive
//             through api.Ports, so the module itself reaches no plugin.
//             The unconfiguredProvider stub moved to stub.go; the cross-provider
//             down-route eligibleSet moved to set.go; governor env parsing moved
//             to config.go.

import (
	"fmt"
	"io"
	"os"

	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/modules/providers/api"
	"github.com/agezt/agezt/kernel/modules/providers/catalog"
)

// Deps bundles everything Boot/Reload need from the daemon. Get and Stderr
// are nil-defaulted (os.Getenv / io.Discard) so tests can inject a map-backed
// environment and capture warnings without touching the process env.
type Deps struct {
	// Catalog is the loaded provider catalog snapshot.
	Catalog *catalog.Catalog
	// Lookup is the chained credential resolver (vault → env → AWS chain).
	Lookup func(string) string
	// BaseDir is the daemon base dir (ChatGPT token store lives under it).
	BaseDir string
	// Get reads configuration environment variables. nil → os.Getenv.
	Get func(string) string
	// Stderr receives non-fatal boot warnings. nil → io.Discard.
	Stderr io.Writer
	// Ports are the wire adapters the engine drives: the compat family, the
	// offline demo provider, the ChatGPT backend and the middleware stack.
	// The composition root fills them in — a module may not import a plugin.
	Ports api.Ports
}

func (d Deps) get(name string) string {
	if d.Get != nil {
		return d.Get(name)
	}
	return os.Getenv(name)
}

func (d Deps) stderr() io.Writer {
	if d.Stderr != nil {
		return d.Stderr
	}
	return io.Discard
}

// ports returns the wired wire adapters, or an error when the composition root
// forgot them. A missing adapter must be LOUD: an engine that quietly ran
// without an adapter is the middleware-drift class this engine was written to
// kill (a provider registered unwrapped looks fine until a reload drops the
// stack), so the wiring error is reported instead of swallowed.
func (d Deps) ports() (api.Ports, error) {
	if d.Ports == nil {
		return nil, fmt.Errorf("provider boot: the wire adapters are not wired (ports are nil)")
	}
	return d.Ports, nil
}

// Result is what Boot hands back to the daemon.
type Result struct {
	// Governor is the constructed routing layer (also the llm.Provider the
	// kernel runs against).
	Governor *governor.Governor
	// Primary is the primary provider's registry name. Equal to
	// UnconfiguredName when no provider is configured — the daemon's
	// first-run nudge keys off exactly this (NOT the model id; the survey
	// found the old `model == "mock"` check fired exactly backwards).
	Primary string
	// Model is the run model for the kernel config ("" when none configured).
	Model string
	// Desc is the human-readable banner description.
	Desc string
	// AuthMode is the primary provider's auth classification.
	AuthMode governor.AuthMode
	// Eligible reads the LIVE cross-provider down-route eligibility set
	// (catalog provider id → registered). Refreshed by Reload; the governor's
	// cross-provider altFinder closure reads the same set.
	Eligible func(providerID string) bool
}

// eligibleSet is the mutex-guarded live eligibility map behind the governor's
// cross-provider down-route altFinder (drift fix: the old implementation
// closed over a plain map built once in buildGovernor, so a reload mutated
// the registry but the down-route search kept the boot-time snapshot).

// Eligible reports whether a catalog provider can serve requests: a supported
// compat family AND resolvable credentials. This is THE eligibility predicate —
// registration (Boot/Reload), the vision sidecar picker, the keyed-model
// delegation predicate, and the council membership all share it (it used to be
// copy-pasted at each site).
//
// The family check belongs to the wire adapter, so it comes in through the
// ports; the credential check belongs to the catalog, which the module owns.
func Eligible(ports api.Ports, entry *catalog.Provider, lookup func(string) string) bool {
	return entry != nil && ports.SupportedFamily(entry.Family()) && entry.HasCredentials(lookup)
}
