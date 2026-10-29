// SPDX-License-Identifier: MIT

package catalog

import (
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

// The catalog family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The catalog is the directory the daemon keeps its
// provider and model catalog in, and four surfaces answer from it: the snapshot
// the routing reads, the store the sync writes, the syncer the discovery
// fetches with, and the refresh that re-reads the directory and rebuilds the
// live provider registry from what it read.
//
// The port holds the kernel's own surfaces as CAPABILITIES — the snapshot, the
// store, the syncer and the refresh — so a family that asks for the catalog
// never names the kernel's verb to get it.
//
// The kernel may be absent, and the port answers for that case: there is no
// catalog to refresh, no store to write into, and no syncer to fetch with.

// Ports are the catalog family's capabilities.
type Ports struct {
	snapshot func() *catalogapi.Catalog
	store    func() catalogapi.Store
	syncer   func() catalogapi.Syncer
	refresh  func() (*catalogapi.Catalog, bool, error)
}

// PortsFromKernel fills the catalog family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no catalog either,
// and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		snapshot: k.Catalog,
		store:    k.CatalogStore,
		syncer:   k.CatalogSyncer,
		refresh:  k.Reload,
	}
}

// Snapshot is the catalog the daemon currently holds.
//
// The name is not "Catalog": a port may not carry the name of the kernel verb
// it stands for, or the adapter that asks the port would be seen reaching for
// the kernel's own verb.
//
// A host with no kernel holds the EMPTY catalog — the same answer the kernel
// gives for a daemon that has never synced.
func (p Ports) Snapshot() *catalogapi.Catalog {
	if p.snapshot == nil {
		return &catalogapi.Catalog{}
	}
	return p.snapshot()
}

// Store is the catalog's on-disk store.
//
// It MAY be absent: the catalog directory is the DAEMON's file, and a host that
// was given no store holds none. The surfaces that write into it refuse
// instead of writing into nothing.
func (p Ports) Store() catalogapi.Store {
	if p.store == nil {
		return nil
	}
	return p.store()
}

// Syncer is the port that fetches a remote catalog.
//
// It MAY be absent, for the same reason the store may be absent: the daemon
// builds the syncer and hands it over. The sync surface refuses instead of
// fetching nothing.
func (p Ports) Syncer() catalogapi.Syncer {
	if p.syncer == nil {
		return nil
	}
	return p.syncer()
}

// Refresh re-reads the catalog from disk and rebuilds the live provider
// registry. It reports whether the provider registry was rebuilt.
//
// The name is not "Reload": a port may not carry the name of the kernel verb
// it stands for.
//
// A host with no kernel refreshes nothing, and says so with the family's own
// refusal rather than a silent no-op.
func (p Ports) Refresh() (*catalogapi.Catalog, bool, error) {
	if p.refresh == nil {
		return nil, false, ErrNoLibrary
	}
	return p.refresh()
}

// Publisher is the catalog family's own narrow view of the bus.
//
// The bus belongs to the bus family, and the catalog family declares only the
// part it uses — the publish — so the host hands over a bus that answers it.
// The view is the CONSUMER's business, not the bus family's: the bus port
// offers more, and this family asks for less.
type Publisher interface {
	Publish(spec event.Spec) (*event.Event, error)
}
