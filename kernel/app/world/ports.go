// SPDX-License-Identifier: MIT

package world

import (
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// The world family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The world model is the daemon's own graph of
// entities and the relations between them — what the operator's world is made
// of — and one surface answers from it: the graph.
//
// The port holds the kernel's own surface as a CAPABILITY — the graph — so a
// family that asks for the world never names the kernel's verb to get it.
//
// The kernel may be absent, and the port answers for that case: there is no
// graph to read from, and the surfaces that read it refuse instead of reading
// from nothing.

// Ports are the world family's capabilities.
type Ports struct {
	graph func() worldmodelapi.Graph
}

// PortsFromKernel fills the world family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no world model
// either, and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{graph: func() worldmodelapi.Graph { return k.World() }}
}

// Graph is the world model — the daemon's entities and their relations.
//
// The name is not "World": a port may not carry the name of the kernel verb it
// stands for, or the adapter that asks the port would be seen reaching for the
// kernel's own verb.
//
// It MAY be absent: the world model is the DAEMON's file, and a host that was
// given none holds none. The surfaces that read it refuse instead of reading
// from nothing.
func (p Ports) Graph() worldmodelapi.Graph {
	if p.graph == nil {
		return nil
	}
	return p.graph()
}
