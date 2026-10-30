// SPDX-License-Identifier: MIT

package api

// The work module's public vocabulary for the OBJECTIVE LIBRARY: the verbs
// the host drives, and the host's ports the module needs to answer them.
//
// It sits beside the task board's vocabulary in the same package: the module
// keeps both libraries, and the host reaches both through the module's face.
//
// It exists because another module may import a module's api package and
// nothing else (archcheck's principle P5). The runtime keeps its own names as
// forwarders, so the host's public API is unchanged while the implementation
// lives in the module.
//
// The ROWS live in the contract layer (kernel/contract/okrapi): the
// agent-facing tools that expose them are plugins, and a plugin may reach the
// contract and the platform layers only.
//
// Rules of the road:
//
//   - Types, interfaces and ports only. No logic that needs a store, a bus or
//     a clock — those are the implementation package's business.
//   - One row, one home. A row both the host and the module name is declared
//     exactly once — in the contract package — and every other side aliases
//     it.

import (
	"github.com/agezt/agezt/kernel/contract/okrapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
)

// Service is the objective library's public face: the goals and their key
// results.
//
// The verbs are the LIBRARY's own, not the daemon's: a host that holds a
// Service can create a goal, add a key result, link a task and archive the
// goal, and it can never reach past the library into the daemon's machinery.
type ObjectiveService interface {
	// Create adds a goal.
	Create(corr string, spec okrapi.CreateSpec) (okrapi.Objective, error)
	// AddKeyResult adds a measurable outcome under a goal.
	AddKeyResult(corr, objID, title string, target int) (okrapi.Objective, error)
	// LinkTask counts a task toward a key result.
	LinkTask(corr, objID, krID, taskID string) (okrapi.Objective, error)
	// UnlinkTask stops counting a task.
	UnlinkTask(corr, objID, krID, taskID string) (okrapi.Objective, error)
	// Archive retires a goal.
	Archive(corr, objID string) (okrapi.Objective, error)
	// Progress computes a goal's progress from the live status of its linked
	// tasks.
	Progress(o okrapi.Objective) okrapi.ObjectiveProgress
	// ProgressOf returns a goal and its live progress.
	ProgressOf(id string) (okrapi.Objective, okrapi.ObjectiveProgress, bool)
	// RecomputeForTask re-rolls every goal that links a task and flips the
	// durable "achieved" signal when it crosses — or regresses below — the
	// target.
	RecomputeForTask(corr, taskID string)
}

// Dependencies are the host's ports the library needs to answer a verb.
//
// Every port is a way to ASK, never a way to reach: the library publishes on
// the host's bus and reads a task's live status, and it holds nothing else of
// the host's.
type ObjectiveDependencies struct {
	// Publish emits one event on the host's bus.
	//
	// The host owns the bus; the library only says what happened to a goal,
	// and the subject and the kind are the library's own. A nil port means
	// the host has no bus, and the library stays quiet rather than
	// publishing into nothing.
	Publish func(spec event.Spec)

	// TaskStatus answers a task's live status.
	//
	// The rollup counts a task when it is DONE, and what "done" means is the
	// library's own rule — so the port answers the STATUS and the library
	// reads the rule. A nil port means the host has no board, and nothing
	// counts toward a goal.
	TaskStatus func(taskID string) (workboardapi.Status, bool)
}
