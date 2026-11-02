// SPDX-License-Identifier: MIT

package workboard

import (
	"github.com/agezt/agezt/kernel/contract/seatapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/modules/work/api"
	"github.com/agezt/agezt/kernel/platform/journalview"
)

// The workboard application's ports.
//
// The APPLICATION owns this: it knows which capabilities the task board
// needs and how each of them answers.
//
// W3.76s: the ports are the board's MODULE and the contract's store port —
// NOT the daemon's verbs. The daemon used to hold sixteen forwarders over the
// module's own operations, and the application bound the DAEMON to reach
// them: a hop that bought nothing, and an import of kernel/runtime this
// package does not need.
//
// The names the application speaks are the CONTRACT's runner port's names
// (claim a workboard task, comment a workboard task) — the board's users say
// them that way, and the module answers in them.
//
// The board's FILE is still the daemon's (W3.47): the daemon opens the file
// and hands the contract's port over. The board's RULES are the module's.

// Ports are the workboard application's capabilities over the task board.
type Ports struct {
	// Reader reads the board.
	//
	// It is the CONTRACT's store port: the host opens the file and hands the
	// port over. It is NIL when the daemon holds no board — the application
	// answers "the task board is not available on this daemon" rather than
	// reading into nothing.
	Reader Reader

	// Store is the board's watch surface: the task and its blocking
	// dependencies.
	Store WatchStore

	// SeatSetter assigns a task to a seat.
	SeatSetter SeatSetter

	// Relations owns the task relations: the link, the retry policy, the
	// dependencies and the stale sweep.
	Relations RelationHost

	// Lifecycle owns the task lifecycle: create, claim, heartbeat, comment,
	// block, fail, unblock, complete, prove and archive.
	Lifecycle LifecycleHost

	// Dispatch owns the claim and link verbs the dispatcher drives.
	Dispatch DispatchHost

	// Correlator mints the correlation of one operation.
	//
	// It is the HOST's own capability and it is not the board's: the board
	// never mints a correlation. It is NIL when the host has none, and the
	// dispatcher answers with an empty correlation rather than calling into
	// nothing.
	Correlator Correlator

	// Execution owns the execution verbs: comment, fail, block, claim, prove
	// and review.
	Execution ExecutionHost

	// SeatIndex reads the seats.
	SeatIndex ExecutionSeats

	// SeatStore is the seat store the daemon validates a seat against.
	//
	// The seat family owns the answer to "is this seat real?"; the workboard
	// family asks it. The store is the contract's port for that answer.
	SeatStore seatapi.Store

	// TaskIndex reads the tasks.
	TaskIndex ExecutionTasks

	// Events reads the event journal the board writes its progress to.
	Events journalview.Reader
}

// PortsFromBoard fills the workboard application's ports from the board's
// runner — the MODULE's service under the contract's runner port — and the
// contract's store port.
//
// The runner is the board's own face: it answers every verb in the contract's
// names, and the application never asks the host to forward.
func PortsFromBoard(runner api.BoardRunner, board workboardapi.HostStore, seats seatapi.Store, events journalview.Reader, correlator Correlator) Ports {
	return Ports{
		Reader:     board,
		Store:      board,
		SeatSetter: board,
		Relations:  runner,
		Lifecycle:  runner,
		Dispatch:   runner,
		Correlator: correlator,
		Execution:  runner,
		SeatIndex:  seats,
		SeatStore:  seats,
		TaskIndex:  board,
		Events:     events,
	}
}
