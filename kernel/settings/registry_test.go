// SPDX-License-Identifier: MIT

package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func sampleSection() Section {
	return Section{
		ID:   "weather-skill",
		Name: "Weather Skill",
		Help: "API access for the weather skill.",
		Fields: []Field{
			{Env: "AGEZT_X_WEATHER_API_KEY", Label: "API key", Type: TypePassword, Secret: true},
			{Env: "AGEZT_X_WEATHER_UNITS", Label: "Units", Type: TypeSelect, Options: []string{"metric", "imperial"}},
		},
	}
}

func TestBuiltinTunnelSettings(t *testing.T) {
	r := NewRegistry(t.TempDir())
	provider, ok := r.FieldByEnv("AGEZT_TUNNEL")
	if !ok {
		t.Fatal("AGEZT_TUNNEL missing from built-in schema")
	}
	if provider.Type != TypeSelect {
		t.Fatalf("AGEZT_TUNNEL type=%q, want %q", provider.Type, TypeSelect)
	}
	want := []string{"", "cloudflare", "cloudflared", "ngrok", "tailscale", "tailscale-funnel", "custom"}
	if !slices.Equal(provider.Options, want) {
		t.Fatalf("AGEZT_TUNNEL options=%v, want %v", provider.Options, want)
	}
	for _, env := range []string{"AGEZT_TUNNEL_TARGET", "AGEZT_TUNNEL_CMD", "AGEZT_TUNNEL_NOTES", "AGEZT_WEB_ALLOWED_HOSTS"} {
		if _, ok := r.FieldByEnv(env); !ok {
			t.Fatalf("%s missing from built-in schema", env)
		}
	}
	if err := Validate(provider, "frpc"); err == nil {
		t.Fatal("AGEZT_TUNNEL should reject unsupported daemon-supervised provider frpc")
	}
}

func TestRegistry_RegisterAndMerge(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	// Built-in only to start.
	if got := len(r.Registered()); got != 0 {
		t.Fatalf("expected 0 registered, got %d", got)
	}
	base := len(r.Sections())
	if base == 0 {
		t.Fatal("expected built-in sections")
	}

	if err := r.Register(sampleSection()); err != nil {
		t.Fatalf("register: %v", err)
	}
	// File landed under schemas/<id>.json.
	if _, err := os.Stat(filepath.Join(dir, SchemaDir, "weather-skill.json")); err != nil {
		t.Fatalf("schema file not written: %v", err)
	}

	secs := r.Sections()
	if len(secs) != base+1 {
		t.Fatalf("expected %d sections, got %d", base+1, len(secs))
	}
	last := secs[len(secs)-1]
	if last.ID != "weather-skill" || last.Source != "weather-skill" {
		t.Fatalf("registered section not tagged: id=%q source=%q", last.ID, last.Source)
	}
	// Registered fields are forced to restart-apply.
	for _, f := range last.Fields {
		if f.Apply != ApplyRestart {
			t.Errorf("field %s apply=%q, want restart", f.Env, f.Apply)
		}
	}
	// FieldByEnv finds a registered field.
	if f, ok := r.FieldByEnv("AGEZT_X_WEATHER_API_KEY"); !ok || !f.Secret {
		t.Errorf("FieldByEnv registered secret: ok=%v secret=%v", ok, f.Secret)
	}
}

func TestReloadBoundaries(t *testing.T) {
	r := NewRegistry(t.TempDir())
	if err := r.Register(sampleSection()); err != nil {
		t.Fatalf("register: %v", err)
	}
	boundaries := ReloadBoundaries(r.Sections())
	if len(boundaries) < 2 {
		t.Fatalf("expected live and restart boundaries, got %+v", boundaries)
	}
	var live, restart []string
	for _, b := range boundaries {
		switch b.Apply {
		case ApplyLive:
			live = b.Envs
		case ApplyRestart:
			restart = b.Envs
		}
	}
	if !contains(live, "AGEZT_PROVIDER") || !contains(live, "AGEZT_MODEL") {
		t.Fatalf("live boundary missing provider/model: %v", live)
	}
	if !contains(restart, "AGEZT_X_WEATHER_API_KEY") {
		t.Fatalf("registered field should require restart: %v", restart)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)
	if err := r.Register(sampleSection()); err != nil {
		t.Fatalf("register: %v", err)
	}
	removed, err := r.Unregister("weather-skill", false)
	if err != nil || !removed {
		t.Fatalf("unregister: removed=%v err=%v", removed, err)
	}
	if got := len(r.Registered()); got != 0 {
		t.Fatalf("expected 0 after unregister, got %d", got)
	}
	// Unregistering a missing id is not an error.
	removed, err = r.Unregister("nope", false)
	if err != nil || removed {
		t.Fatalf("unregister missing: removed=%v err=%v", removed, err)
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func TestRegistry_LockedSectionNeedsForce(t *testing.T) {
	r := NewRegistry(t.TempDir())
	sec := sampleSection()
	sec.ID = "locked-skill"
	sec.Locked = true
	if err := r.Register(sec); err != nil {
		t.Fatalf("register: %v", err)
	}
	// Without force the locked section is refused.
	if removed, err := r.Unregister("locked-skill", false); err == nil || removed {
		t.Fatalf("expected locked section to refuse unregister: removed=%v err=%v", removed, err)
	}
	// With force it goes.
	if removed, err := r.Unregister("locked-skill", true); err != nil || !removed {
		t.Fatalf("force unregister: removed=%v err=%v", removed, err)
	}
}

func TestRegistry_RejectsShadowingBuiltin(t *testing.T) {
	r := NewRegistry(t.TempDir())
	sec := sampleSection()
	sec.Fields = append(sec.Fields, Field{Env: "AGEZT_ALLOW_ALL", Label: "pwn", Type: TypeBool})
	if err := r.Register(sec); err == nil {
		t.Fatal("expected register to reject a field shadowing AGEZT_ALLOW_ALL")
	}
}

func TestRegistry_RejectsBadInput(t *testing.T) {
	r := NewRegistry(t.TempDir())
	cases := map[string]Section{
		"bad id":            {ID: "Bad Id", Name: "x", Fields: []Field{{Env: "AGEZT_X_A", Type: TypeText}}},
		"no fields":         {ID: "empty", Name: "x"},
		"non-namespaced":    {ID: "ns", Name: "x", Fields: []Field{{Env: "OPENAI_KEY", Type: TypeText}}},
		"not agezt prefix":  {ID: "ns2", Name: "x", Fields: []Field{{Env: "AGEZTX", Type: TypeText}}},
		"unknown type":      {ID: "ty", Name: "x", Fields: []Field{{Env: "AGEZT_X_A", Type: "wat"}}},
		"select no options": {ID: "se", Name: "x", Fields: []Field{{Env: "AGEZT_X_A", Type: TypeSelect}}},
		"dup env":           {ID: "dup", Name: "x", Fields: []Field{{Env: "AGEZT_X_A", Type: TypeText}, {Env: "AGEZT_X_A", Type: TypeText}}},
	}
	for name, sec := range cases {
		if err := r.Register(sec); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

// A hand-edited file that sneaks a colliding/non-namespaced field past Register
// must still have that field dropped at read time (built-in wins).
func TestRegistry_SectionsDropsColliding(t *testing.T) {
	dir := t.TempDir()
	schemas := filepath.Join(dir, SchemaDir)
	if err := os.MkdirAll(schemas, 0755); err != nil {
		t.Fatal(err)
	}
	sec := Section{
		ID:   "sneaky",
		Name: "Sneaky",
		Fields: []Field{
			{Env: "AGEZT_X_OK", Label: "ok", Type: TypeText},
			{Env: "AGEZT_ALLOW_ALL", Label: "pwn", Type: TypeBool}, // collides with built-in
			{Env: "OPENAI_KEY", Label: "escape", Type: TypeText},   // not namespaced
		},
	}
	raw, _ := json.MarshalIndent(sec, "", "  ")
	if err := os.WriteFile(filepath.Join(schemas, "sneaky.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(dir)
	var found *Section
	for _, s := range r.Sections() {
		if s.ID == "sneaky" {
			cp := s
			found = &cp
		}
	}
	if found == nil {
		t.Fatal("sneaky section missing")
	}
	if len(found.Fields) != 1 || found.Fields[0].Env != "AGEZT_X_OK" {
		t.Fatalf("expected only AGEZT_X_OK to survive, got %+v", found.Fields)
	}
	// And it never shadowed the real built-in: AGEZT_ALLOW_ALL still resolves to
	// the built-in bool field, not the sneaky registered one.
	if f, ok := r.FieldByEnv("AGEZT_ALLOW_ALL"); !ok || f.Type != TypeBool {
		t.Errorf("AGEZT_ALLOW_ALL did not resolve to the built-in: ok=%v field=%+v", ok, f)
	}
}
