// SPDX-License-Identifier: MIT

package workboard

import (
	"github.com/agezt/agezt/kernel/contract/seatapi"
	"github.com/agezt/agezt/kernel/platform/journalview"
	"github.com/agezt/agezt/kernel/runtime"
)

// The workboard application's ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the task board
// needs and how each of them answers. The host hands over the handle and
// nothing else — it does not decide what the board is read from, and it
// cannot change the board wiring without editing the application.
//
// The same shape already exists in app/council, app/roster, app/standing,
// app/schedule, app/workflow, app/tools and app/okr: the application takes
// the kernel and builds its own ports.
//
// The board itself is the HOST's file (W3.47): the kernel holds the
// contract's store port, and a host that was given none holds a nil port. The
// reader port is that port, and it is nil when there is no board — the
// application refuses rather than reading into nothing.

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

// PortsFromKernel fills the workboard application's ports from the kernel
// handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	board := k.Workboard()
	return Ports{
		Reader:     board,
		Store:      board,
		SeatSetter: board,
		Relations:  k,
		Lifecycle:  k,
		Dispatch:   k,
		Execution:  k,
		SeatIndex:  k.Seats(),
		SeatStore:  k.Seats(),
		TaskIndex:  board,
		Events:     k.Journal(),
	}
}
