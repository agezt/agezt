// SPDX-License-Identifier: MIT

package market

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/mcp"
)

// PackManifest is the AUTHORING shape of a pack on disk: skills are referenced
// as directories (each a SKILL.md bundle) rather than inlined, so a publisher
// edits real files. BuildPackFromDir compiles it into the wire Pack (skills
// inlined + resources embedded) that the marketplace serves and installs.
//
// Layout:
//
//	mypack/
//	  pack.json                 # this manifest
//	  skills/<name>/SKILL.md     # one dir per referenced skill (+ reference/ scripts/)
type PackManifest struct {
	Name             string       `json:"name"`
	Version          string       `json:"version"`
	Description      string       `json:"description,omitempty"`
	Author           string       `json:"author,omitempty"`
	Category         string       `json:"category,omitempty"`
	Tags             []string     `json:"tags,omitempty"`
	Keywords         []string     `json:"keywords,omitempty"`
	SkillDirs        []string     `json:"skills,omitempty"` // relative dirs each holding a SKILL.md
	MCPServers       []mcp.Server `json:"mcp_servers,omitempty"`
	ToolRequirements []string     `json:"tool_requirements,omitempty"`
}

// maxResourceBytes caps a single bundled resource file (mirrors the skill
// registry's per-file ceiling) so a publish dir can't embed a huge blob.
const maxResourceBytes = 4 * 1024 * 1024

// BuildPackFromDir reads <dir>/pack.json and compiles it into a wire Pack:
// every referenced skill dir's SKILL.md is inlined and its reference/scripts
// files embedded as resources. The result is Validate()d before return, so a
// malformed authoring dir fails loudly rather than publishing a broken pack.
func BuildPackFromDir(dir string) (Pack, error) {
	manifestPath := filepath.Join(dir, "pack.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return Pack{}, fmt.Errorf("market: read %s: %w", manifestPath, err)
	}
	var man PackManifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return Pack{}, fmt.Errorf("market: parse pack.json: %w", err)
	}
	p := Pack{
		Name:             man.Name,
		Version:          man.Version,
		Description:      man.Description,
		Author:           man.Author,
		Category:         man.Category,
		Tags:             man.Tags,
		Keywords:         man.Keywords,
		MCPServers:       man.MCPServers,
		ToolRequirements: man.ToolRequirements,
	}
	for _, rel := range man.SkillDirs {
		if err := safeRelPath(rel); err != nil {
			return Pack{}, fmt.Errorf("market: skill dir %q: %w", rel, err)
		}
		ps, err := buildPackSkill(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return Pack{}, err
		}
		p.Skills = append(p.Skills, ps)
	}
	if err := p.Validate(); err != nil {
		return Pack{}, err
	}
	return p, nil
}

// buildPackSkill reads a skill bundle dir into a PackSkill: SKILL.md (required)
// plus every other file as a resource keyed by its forward-slash relative path.
func buildPackSkill(skillDir string) (PackSkill, error) {
	mdPath := filepath.Join(skillDir, "SKILL.md")
	md, err := os.ReadFile(mdPath)
	if err != nil {
		return PackSkill{}, fmt.Errorf("market: read %s: %w", mdPath, err)
	}
	resources := map[string][]byte{}
	err = filepath.WalkDir(skillDir, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(skillDir, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if rel == "SKILL.md" {
			return nil // inlined separately
		}
		if err := safeRelPath(rel); err != nil {
			return fmt.Errorf("resource %q: %w", rel, err)
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		if info.Size() > maxResourceBytes {
			return fmt.Errorf("resource %q exceeds %d bytes", rel, maxResourceBytes)
		}
		b, rerr2 := os.ReadFile(path)
		if rerr2 != nil {
			return rerr2
		}
		resources[rel] = b
		return nil
	})
	if err != nil {
		return PackSkill{}, fmt.Errorf("market: bundle %s: %w", skillDir, err)
	}
	if len(resources) == 0 {
		resources = nil
	}
	return PackSkill{SkillMD: string(md), Resources: resources}, nil
}

// Publish writes a (optionally signed) pack into a statically-hostable
// marketplace directory: <out>/packs/<name>.json plus an upserted entry in
// <out>/marketplace.json. An existing marketplace.json is loaded and merged, so
// repeated publishes build up one catalogue. priv may be nil (unsigned).
func Publish(p Pack, outDir, marketplaceName string, priv ed25519.PrivateKey, nowMS int64) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if priv != nil {
		sig, err := SignPack(p, priv, nowMS)
		if err != nil {
			return err
		}
		p.Signature = sig
	}
	packsDir := filepath.Join(outDir, "packs")
	if err := os.MkdirAll(packsDir, 0o755); err != nil {
		return err
	}
	packBytes, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(packsDir, p.Name+".json"), packBytes, 0o644); err != nil {
		return err
	}

	// Load-or-create the index, then upsert this pack's entry.
	idxPath := filepath.Join(outDir, "marketplace.json")
	mp := Marketplace{Name: marketplaceName, FormatVersion: FormatVersion}
	if existing, rerr := os.ReadFile(idxPath); rerr == nil {
		_ = json.Unmarshal(existing, &mp) // a corrupt index is overwritten fresh
	}
	if mp.Name == "" {
		mp.Name = marketplaceName
	}
	mp.FormatVersion = FormatVersion
	mp.GeneratedUnixMS = nowMS
	entry := p.Entry("packs/" + p.Name + ".json")
	entry.Signed = p.Signature != nil
	replaced := false
	for i := range mp.Packs {
		if mp.Packs[i].Name == p.Name {
			mp.Packs[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		mp.Packs = append(mp.Packs, entry)
	}
	sort.Slice(mp.Packs, func(i, j int) bool { return mp.Packs[i].Name < mp.Packs[j].Name })
	idxBytes, err := json.MarshalIndent(mp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(idxPath, idxBytes, 0o644)
}

// GenerateKeypair returns a fresh Ed25519 keypair as lowercase hex strings
// (pub, priv) for `agt market keygen` — the priv signs packs, the pub is pinned
// on a Source so consumers verify the publisher.
func GenerateKeypair() (pubHex, privHex string, err error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return "", "", err
	}
	return hex.EncodeToString(pub), hex.EncodeToString(priv.Seed()), nil
}

// PrivateKeyFromHex rebuilds a signing key from a 32-byte seed hex (what keygen
// emits) or a full 64-byte private-key hex.
func PrivateKeyFromHex(h string) (ed25519.PrivateKey, error) {
	b, err := hex.DecodeString(strings.TrimSpace(h))
	if err != nil {
		return nil, fmt.Errorf("market: bad key hex: %w", err)
	}
	switch len(b) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(b), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(b), nil
	default:
		return nil, fmt.Errorf("market: key must be %d-byte seed or %d-byte private key (got %d)", ed25519.SeedSize, ed25519.PrivateKeySize, len(b))
	}
}
