// SPDX-License-Identifier: MIT

package skill

// Built-in skill seeding (M852): the bundles baked into the binary (the
// sibling builtins package) are materialized into the Forge at startup, so
// capabilities like full browser automation work out of the box — no operator
// install, no agent having to author the skill first.
//
// Seeding is idempotent: skills are content-addressed by (name, body), so a
// re-seed of an unchanged bundle dedupes onto the existing record and just
// refreshes its on-disk files; a changed bundle (new binary) becomes a new
// version.

import (
	"fmt"

	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/modules/skills/builtins"
)

// ForgeAPI is the slice of the Forge the seeder needs — an interface so it is
// testable with a fake and so the seeder does not depend on the whole store.
type ForgeAPI interface {
	Create(corr string, spec CreateSpec) (Skill, bool, error)
	Promote(corr, id string) (Status, error)
	Get(id string) (Skill, bool, error)
}

// Seeded names one bundle that was installed, with its final lifecycle status.
type Seeded struct {
	Name    string
	ID      string
	Status  Status
	Created bool
}

// builtins is the module's implementation of the contract's Builtins port —
// the built-in bundles baked into the binary.
type builtinsImpl struct{}

// NewBuiltins returns the port through which the built-in bundles are read.
// The bytes are an implementation (they live in the embedded builtins
// package), so a plugin that publishes them — the marketplace's built-in
// catalogue — reads them through this port instead of naming the module.
func NewBuiltins() skillapi.Builtins { return builtinsImpl{} }

// Names returns the embedded built-in bundle directory names.
func (builtinsImpl) Names() []string { return builtins.Names() }

// Bundle returns one embedded built-in bundle's raw SKILL.md plus its
// resource files (relative path → bytes), so other packagers — e.g. the
// marketplace's built-in "Official" seed — can wrap a built-in skill without
// duplicating its bytes.
func (builtinsImpl) Bundle(name string) (skillMD []byte, resources map[string][]byte, err error) {
	return builtins.Bundle(name)
}

// SeedBuiltins installs every embedded bundle into the Forge and promotes each
// to active. It is best-effort per bundle: an error on one is returned but does
// not stop the others. corr is the journaling correlation (use "" for boot).
func SeedBuiltins(f ForgeAPI, corr string) ([]Seeded, error) {
	var out []Seeded
	var firstErr error
	for _, dir := range builtins.Names() {
		s, err := seedOne(f, corr, dir)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("seed %s: %w", dir, err)
			}
			continue
		}
		out = append(out, s)
	}
	return out, firstErr
}

// seedOne reads one embedded bundle dir (its SKILL.md + the rest as resources),
// creates the skill, and promotes it to active.
func seedOne(f ForgeAPI, corr, dir string) (Seeded, error) {
	mdRaw, resources, err := builtins.Bundle(dir)
	if err != nil {
		return Seeded{}, err
	}
	md, err := ParseSkillMD(mdRaw)
	if err != nil {
		return Seeded{}, err
	}

	sk, created, err := f.Create(corr, CreateSpec{
		Name:          md.Name,
		Description:   md.Description,
		Triggers:      md.Triggers,
		Body:          md.Body,
		ToolsRequired: md.ToolsRequired,
		Resources:     resources,
	})
	if err != nil {
		return Seeded{}, err
	}

	// Promote to active so it is immediately in the retrieval pool — but never
	// resurrect a skill that was pulled OUT of production. A quarantined
	// built-in stays quarantined: the seeder's job is to make the built-ins
	// AVAILABLE, not to re-open a lifecycle decision an operator (or the
	// regression guard) already made. Before this, every boot promoted a
	// quarantined built-in straight back into the retrieval pool.
	//
	// Idempotent: stop once active (or once a promote makes no progress);
	// ignore the terminal "already active" error.
	status := sk.Status
	for i := 0; i < 3 && status != StatusActive; i++ {
		if status == StatusQuarantined || status == StatusArchived {
			break
		}
		next, perr := f.Promote(corr, sk.ID)
		if perr != nil {
			break
		}
		if next == status {
			break
		}
		status = next
	}
	return Seeded{Name: md.Name, ID: sk.ID, Status: status, Created: created}, nil
}
