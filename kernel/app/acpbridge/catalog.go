// SPDX-License-Identifier: MIT

// Package acpbridge answers the ACP bridge tool's contract port.
//
// The acp_agent tool is a PLUGIN: it may not name the catalogue module that
// knows which ACP agents this host has installed. So the module's three
// questions — is any agent installed, which slugs are installed, and which
// launch command does a slug map to — answer through the contract's port
// (kernel/contract/acpapi), and the daemon hands this adapter over.
//
// The adapter is a thin, honest forwarding layer: the catalogue's rules (the
// curated agent list, the host detection and the slug-only resolution that
// keeps a prompt-injected selector from becoming a shell line) stay with the
// module that owns them. What moves here is only the ANSWERING.
package acpbridge

import (
	"github.com/agezt/agezt/kernel/contract/acpapi"
	"github.com/agezt/agezt/kernel/modules/extensions/acpcatalog"
)

// Catalog answers the contract's port from the ACP catalogue module.
//
// It is a value type with no state: the catalogue's answers depend on the
// host, not on the adapter.
type Catalog struct{}

// NewCatalog returns the adapter that answers the catalogue port.
func NewCatalog() *Catalog { return &Catalog{} }

// AnyInstalled reports whether at least one catalogued ACP agent is present
// on this host.
func (Catalog) AnyInstalled() bool { return acpcatalog.AnyInstalled() }

// InstalledSlugs returns the slugs of the catalogued agents present on this
// host.
func (Catalog) InstalledSlugs() []string { return acpcatalog.InstalledSlugs() }

// ResolveCommand turns the bridge tool's agent selector into a launch command.
// The module owns the rule that a selector is a slug and never a raw shell
// line — see the port's contract.
func (Catalog) ResolveCommand(ref, fallback string) (string, bool) {
	return acpcatalog.ResolveCommand(ref, fallback)
}

// The adapter answers the contract's port — that is the whole of its job.
var _ acpapi.Catalog = Catalog{}
