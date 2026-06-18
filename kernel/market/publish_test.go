// SPDX-License-Identifier: MIT

package market

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writePackDir lays out a minimal pack authoring dir (pack.json + one skill
// bundle with a resource) and returns its path.
func writePackDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	manifest := PackManifest{
		Name:             "demo-pack",
		Version:          "2.1.0",
		Description:      "A demo pack for the publish round-trip.",
		Category:         "test",
		Tags:             []string{"demo"},
		SkillDirs:        []string{"skills/demo"},
		ToolRequirements: []string{"jq"},
	}
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "pack.json"), mb, 0o644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(dir, "skills", "demo", "reference")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "demo", "SKILL.md"), []byte(sampleSkillMD), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "tips.md"), []byte("cite everything"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBuildPackFromDir(t *testing.T) {
	p, err := BuildPackFromDir(writePackDir(t))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if p.Name != "demo-pack" || p.Version != "2.1.0" {
		t.Fatalf("metadata wrong: %+v", p)
	}
	if len(p.Skills) != 1 {
		t.Fatalf("want 1 skill, got %d", len(p.Skills))
	}
	if got := string(p.Skills[0].Resources["reference/tips.md"]); got != "cite everything" {
		t.Fatalf("resource not bundled: %q", got)
	}
}

func TestPublishRoundTripSignedAndConsumable(t *testing.T) {
	src := writePackDir(t)
	p, err := BuildPackFromDir(src)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	pubHex, privHex, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	priv, err := PrivateKeyFromHex(privHex)
	if err != nil {
		t.Fatalf("key parse: %v", err)
	}

	out := t.TempDir()
	if err := Publish(p, out, "acme", priv, 42); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// The index lists the pack with a source path + signed flag.
	idx, err := os.ReadFile(filepath.Join(out, "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mp Marketplace
	if err := json.Unmarshal(idx, &mp); err != nil {
		t.Fatal(err)
	}
	if len(mp.Packs) != 1 || !mp.Packs[0].Signed || mp.Packs[0].Source != "packs/demo-pack.json" {
		t.Fatalf("index entry wrong: %+v", mp.Packs)
	}

	// The published pack body verifies against the generated key.
	body, err := os.ReadFile(filepath.Join(out, "packs", "demo-pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	var published Pack
	if err := json.Unmarshal(body, &published); err != nil {
		t.Fatal(err)
	}
	signed, verr := VerifyPack(published, pubHex)
	if verr != nil || !signed {
		t.Fatalf("published pack should verify against its key: signed=%v err=%v", signed, verr)
	}

	// Re-publishing a second pack merges into the same index (upsert + sort).
	p2 := p
	p2.Name = "another-pack"
	if err := Publish(p2, out, "acme", nil, 43); err != nil {
		t.Fatalf("second publish: %v", err)
	}
	idx2, _ := os.ReadFile(filepath.Join(out, "marketplace.json"))
	var mp2 Marketplace
	_ = json.Unmarshal(idx2, &mp2)
	if len(mp2.Packs) != 2 || mp2.Packs[0].Name != "another-pack" {
		t.Fatalf("merge/sort wrong: %+v", mp2.Packs)
	}
}

func TestBuildPackFromDirRejectsTraversalSkillDir(t *testing.T) {
	dir := t.TempDir()
	man := PackManifest{Name: "bad", Version: "1.0.0", SkillDirs: []string{"../escape"}}
	mb, _ := json.MarshalIndent(man, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "pack.json"), mb, 0o644)
	if _, err := BuildPackFromDir(dir); err == nil {
		t.Fatal("must reject a traversal skill dir")
	}
}
