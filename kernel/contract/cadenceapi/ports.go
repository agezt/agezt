// SPDX-License-Identifier: MIT

package cadenceapi

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/contract/journalapi"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
)

// SystemTaskPorts is the capability set the cadence module's system tasks run
// on.
//
// The system tasks are the daemon's own recurring maintenance jobs — the
// catalog sync, the artifact collection, the memory clean and tidy, the log
// clean, the graveyard scan, the profile distill. They used to run against a
// live kernel, which made the cadence module depend on the runs module. The
// tasks still need the daemon's capabilities; what changes is HOW they ask
// for them: through the contract's ports, so the module names the contract
// and never the kernel.
//
// A capability the daemon does not have is a NIL field, and a nil field is a
// refusal and not an empty answer: "this daemon has no memory" and "the
// memory is empty" are different facts, so the task that needs the memory
// says so instead of reporting a clean store it never looked at.
//
// The set is deliberately NARROW. The artifact index is a ten-verb host port
// and two of its verbs delete bytes; the collect task needs one of them, so
// it gets one. The memory is the whole manager — recall, write, distil, the
// tool — and the clean and tidy tasks need two hygiene verbs, so they get the
// MAINTAINER half.
//
// The rules of the road:
//   - Rows and ports only. No clock, no file, no bus.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
type SystemTaskPorts struct {
	// Publish journals the task's summary event.
	//
	// The port answers whether the event landed; the tasks ignore that
	// answer the way they always have — a summary event that could not be
	// written must not fail the task it summarises.
	Publish eventapi.Publisher

	// Memory is the memory half the clean and tidy tasks maintain. It is the
	// contract's MAINTAINER port — the hygiene verbs — not the whole manager:
	// these tasks do not recall, do not write and do not distil.
	Memory memoryapi.Maintainer

	// DistillProfile synthesizes the operator profile from the accumulated
	// memory. It is the one task that asks a model a question, so the port
	// takes the caller's context and correlation.
	DistillProfile func(ctx context.Context, corr string) (memoryapi.ProfileReport, error)

	// RosterList lists the agent profiles the graveyard scan reads.
	RosterList func() []agentapi.Profile

	// CollectArtifacts reclaims the artifact entries older than the cutoff
	// and answers how many were collected and how many bytes they freed.
	CollectArtifacts func(olderThanMs int64) (collected int, bytes int64)

	// ScanJournal walks the daemon's durable history — the log task's read
	// side — and answers the chain's head.
	ScanJournal journalapi.Scanner

	// CatalogSyncer downloads the provider and model catalog.
	CatalogSyncer catalogapi.Syncer

	// CatalogStore persists the downloaded catalog.
	CatalogStore catalogapi.Store

	// Reload re-reads the catalog and rebuilds the provider registry, and
	// answers whether the providers were rebuilt.
	Reload func() (*catalogapi.Catalog, bool, error)
}
