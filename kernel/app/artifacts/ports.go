// SPDX-License-Identifier: MIT

package artifacts

import (
	"github.com/agezt/agezt/kernel/runtime"
)

// The artifacts family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The artifact store is the content-addressed store
// the loop offloads oversized tool outputs into, and the artifact index is the
// browsable, deletable metadata over it. Two surfaces answer from the pair, and
// the family asks for both.
//
// The port holds the kernel's own surfaces as CAPABILITIES — the store and the
// index — so a family that asks for the artifacts never names the kernel's verb
// to get it.
//
// The kernel may be absent, and the port answers for that case: there is no
// store to read from and no index to browse.

// Ports are the artifacts family's capabilities.
type Ports struct {
	store func() BlobStore
	index func() Index
}

// PortsFromKernel fills the artifacts family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no artifact store
// either, and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		store: func() BlobStore { return k.Artifacts() },
		index: func() Index { return k.ArtifactIndex() },
	}
}

// Store is the content-addressed artifact store.
//
// The name is not "Artifacts" or "ArtifactStore": a port may not carry the name
// of the kernel verb it stands for, or the adapter that asks the port would be
// seen reaching for the kernel's own verb.
//
// It MAY be absent: the store is the DAEMON's file, and a host that was given
// none holds none. The surfaces that read from it refuse instead of reading
// from nothing.
func (p Ports) Store() BlobStore {
	if p.store == nil {
		return nil
	}
	return p.store()
}

// Index is the metadata index over the artifact store.
//
// It MAY be absent, for the same reason the store may be absent.
func (p Ports) Index() Index {
	if p.index == nil {
		return nil
	}
	return p.index()
}
