// SPDX-License-Identifier: MIT

// Package builtins ships the skill bundles baked into the binary — the
// agentskills.io shape (M847): a SKILL.md plus reference files and scripts,
// embedded here and materialized into the skill store by the skills module.
//
// The bundles are the module's own built-ins: the module seeds them into the
// Forge at startup, so capabilities like full browser automation work out of
// the box — no operator install, no agent having to author the skill first
// (M852). The seeding itself lives in the module (skill.SeedBuiltins); this
// package holds the bytes and the directory listing.
//
// Other packagers — e.g. the marketplace's built-in "Official" seed — read the
// same bytes through skill.BuiltinBundle, so a built-in skill is never
// duplicated.
package builtins

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:embed browseruse computeruse dataanalysis dockerservices gitops webresearch pdftools imagetools sqldb archivetools httpapi emailtools sshremote cryptotools calendartools officedocs
var bundles embed.FS

// builtinBundles lists the embedded bundle directories to seed.
var builtinBundles = []string{"browseruse", "computeruse", "dataanalysis", "dockerservices", "gitops", "webresearch", "pdftools", "imagetools", "sqldb", "archivetools", "httpapi", "emailtools", "sshremote", "cryptotools", "calendartools", "officedocs"}

// Names returns the embedded built-in bundle directory names.
func Names() []string { return append([]string(nil), builtinBundles...) }

// Bundle returns one embedded bundle's raw SKILL.md plus its resource files
// (relative path → bytes), so other packagers can wrap a built-in skill
// without duplicating its bytes.
func Bundle(name string) (skillMD []byte, resources map[string][]byte, err error) {
	mdRaw, err := bundles.ReadFile(path.Join(name, "SKILL.md"))
	if err != nil {
		return nil, nil, fmt.Errorf("read SKILL.md: %w", err)
	}
	resources = map[string][]byte{}
	walkErr := fs.WalkDir(bundles, name, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, name+"/")
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
		return nil, nil, walkErr
	}
	if len(resources) == 0 {
		resources = nil
	}
	return mdRaw, resources, nil
}
