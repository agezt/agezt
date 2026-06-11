// SPDX-License-Identifier: MIT

// Package builtinskills ships skill bundles baked into the binary and seeds them
// into the daemon's Forge at startup, so capabilities like full browser
// automation work out of the box — no operator install, no agent having to
// author the skill first (M852). Each bundle is the agentskills.io shape (M847):
// a SKILL.md plus reference files and scripts, embedded here and materialized
// into the skill store on boot.
//
// Seeding is idempotent: skills are content-addressed by (name, body), so a
// re-seed of an unchanged bundle dedupes onto the existing record and just
// refreshes its on-disk files; a changed bundle (new binary) becomes a new
// version. The seeded skill is promoted to active so it is immediately in the
// retrieval pool.
package builtinskills

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/agezt/agezt/kernel/skill"
)

//go:embed browseruse computeruse dataanalysis dockerservices
var bundles embed.FS

// Forge is the slice of *skill.Forge the seeder needs — an interface so it is
// testable with a fake and so this package doesn't pull the kernel runtime.
type Forge interface {
	Create(corr string, spec skill.CreateSpec) (skill.Skill, bool, error)
	Promote(corr, id string) (skill.Status, error)
	Get(id string) (skill.Skill, bool, error)
}

// Seeded names one bundle that was installed, with its final lifecycle status.
type Seeded struct {
	Name    string
	ID      string
	Status  skill.Status
	Created bool
}

// builtinBundles lists the embedded bundle directories to seed.
var builtinBundles = []string{"browseruse", "computeruse", "dataanalysis", "dockerservices"}

// SeedAll installs every embedded bundle into the Forge and promotes each to
// active. It is best-effort per bundle: an error on one is returned but does not
// stop the others. corr is the journaling correlation (use "" for boot).
func SeedAll(f Forge, corr string) ([]Seeded, error) {
	var out []Seeded
	var firstErr error
	for _, dir := range builtinBundles {
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
func seedOne(f Forge, corr, dir string) (Seeded, error) {
	mdRaw, err := bundles.ReadFile(path.Join(dir, "SKILL.md"))
	if err != nil {
		return Seeded{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	md, err := skill.ParseSkillMD(mdRaw)
	if err != nil {
		return Seeded{}, err
	}

	resources := map[string][]byte{}
	walkErr := fs.WalkDir(bundles, dir, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, dir+"/")
		if rel == "SKILL.md" {
			return nil // the body, not a resource
		}
		data, rerr := bundles.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		resources[rel] = data
		return nil
	})
	if walkErr != nil {
		return Seeded{}, walkErr
	}

	sk, created, err := f.Create(corr, skill.CreateSpec{
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

	// Promote to active so it is immediately in the retrieval pool. Idempotent:
	// stop once active (or once a promote makes no progress); ignore the terminal
	// "already active" error.
	status := sk.Status
	for i := 0; i < 3 && status != skill.StatusActive; i++ {
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
