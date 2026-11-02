// SPDX-License-Identifier: MIT

package okr

import (
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
)

// The objective application's ports.
//
// The APPLICATION owns this: it knows which capabilities the objectives need
// and how each of them answers.
//
// W3.76r: the ports are the WORK MODULE's service and the contract's store
// port — NOT the daemon's verbs. The daemon used to hold six forwarders over
// the module's own operations (CreateObjective, AddObjectiveKeyResult,
// LinkObjectiveTask, UnlinkObjectiveTask, ArchiveObjective, Rollup), and the
// application bound the DAEMON to reach them: a hop that bought nothing, and
// an import of kernel/runtime this package does not need.
//
// The library's FILE is still the daemon's (W3.53): the daemon opens it and
// hands the contract's port over. The library's RULES are the module's.

// Ports are the objective application's capabilities over the objectives.
type Ports struct {
	// Reader reads the objective library.
	//
	// It is the CONTRACT's store port: the daemon opens the file and hands the
	// port over. It is NIL when the daemon holds no objective library — the
	// application answers "the objective library is not available on this
	// daemon" rather than reading into nothing.
	Reader Reader

	// Library is the work module's objective library service: the mutations
	// (create, key result, link, unlink, archive) and the rollup rule.
	//
	// It is NIL when the daemon holds no library service — the application
	// refuses rather than calling into nothing.
	Library workapi.ObjectiveService
}

// PortsFromLibrary fills the objective application's ports from the work
// module's service and the contract's store port.
func PortsFromLibrary(library workapi.ObjectiveService, reader Reader) Ports {
	return Ports{
		Reader:  reader,
		Library: library,
	}
}
