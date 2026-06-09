// SPDX-License-Identifier: MIT

package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SchemaDir is the subdirectory under <baseDir> holding registered schema
// sections — one JSON file per section (<id>.json). Skills/plugins "drop" a
// section here (via the control plane, `agt config schema register`, or the
// `config` tool) and the Config Center merges it with the built-in schema. This
// is the disk-merge pattern from kernel/catalog applied to config schema.
const SchemaDir = "schemas"

// envNamePattern constrains registered field env-var names: an AGEZT_ prefix,
// then uppercase / digits / underscore. Keeps registered keys namespaced and
// shell-safe, and (together with the built-in reserved set) prevents a skill
// from shadowing a core setting.
var envNamePattern = regexp.MustCompile(`^AGEZT_[A-Z0-9_]+$`)

// slugPattern constrains a section id to a filesystem- and URL-safe slug.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Registry is the single source of truth for the editable configuration surface:
// the compiled-in built-in sections merged with on-disk registered sections from
// <baseDir>/schemas/*.json. Built-in always wins on collision. The registry holds
// no state — it reads the directory on each call, so a newly-registered section
// is visible immediately (the dir is tiny). Safe for concurrent use.
type Registry struct {
	dir string
}

// NewRegistry returns a Registry rooted at <baseDir>/schemas.
func NewRegistry(baseDir string) *Registry {
	return &Registry{dir: filepath.Join(baseDir, SchemaDir)}
}

// Dir is the schemas directory (it may not exist yet).
func (r *Registry) Dir() string { return r.dir }

// builtinEnvSet returns the set of env-var names owned by the built-in schema.
// Registered fields may not shadow these.
func builtinEnvSet() map[string]bool {
	set := map[string]bool{}
	for _, sec := range builtinSections() {
		for _, f := range sec.Fields {
			set[f.Env] = true
		}
	}
	return set
}

// Sections returns the merged configuration surface: built-in sections first,
// then registered sections sorted by id. As a safety net (even against a
// hand-edited schema file), any registered field that is not namespaced
// (AGEZT_*) or that collides with a built-in env name is dropped — built-in
// always wins — and a registered section left with no usable fields is omitted.
func (r *Registry) Sections() []Section {
	out := builtinSections()
	reserved := builtinEnvSet()
	for _, sec := range r.Registered() {
		kept := make([]Field, 0, len(sec.Fields))
		for _, f := range sec.Fields {
			if reserved[f.Env] || !envNamePattern.MatchString(f.Env) {
				continue // never let a registered field shadow or escape its namespace
			}
			kept = append(kept, f)
		}
		if len(kept) == 0 {
			continue
		}
		sec.Fields = kept
		out = append(out, sec)
	}
	return out
}

// Registered returns the on-disk registered sections (sorted by id), each tagged
// with Source = its id and every field normalised to ApplyRestart (only the
// built-in provider/model hot-reload; skill/plugin config is read at the plugin's
// own startup). Structurally broken files (unparseable, no slug id, no fields)
// are skipped silently so one bad file can't break the whole Config Center;
// per-field safety (namespacing, no shadowing) is enforced in Sections(), and
// Register() validates strictly on write.
func (r *Registry) Registered() []Section {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil // missing dir = nothing registered
	}
	var out []Section
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(r.dir, e.Name()))
		if err != nil {
			continue
		}
		var sec Section
		if err := json.Unmarshal(raw, &sec); err != nil {
			continue
		}
		if sec.ID == "" {
			sec.ID = strings.TrimSuffix(e.Name(), ".json")
		}
		if !slugPattern.MatchString(sec.ID) || len(sec.Fields) == 0 {
			continue
		}
		for i := range sec.Fields {
			sec.Fields[i].Apply = ApplyRestart
		}
		sec.Source = sec.ID
		out = append(out, sec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// FieldByEnv finds a field by its env name across the merged surface.
func (r *Registry) FieldByEnv(env string) (Field, bool) {
	for _, sec := range r.Sections() {
		for _, f := range sec.Fields {
			if f.Env == env {
				return f, true
			}
		}
	}
	return Field{}, false
}

// Register validates a section and persists it to <dir>/<id>.json (atomic, 0600),
// replacing any existing section with the same id. Returns a descriptive error if
// the section violates the registered-schema contract (slug id, namespaced fields,
// no shadowing of built-ins).
func (r *Registry) Register(sec Section) error {
	if err := validateSection(sec, builtinEnvSet()); err != nil {
		return err
	}
	for i := range sec.Fields {
		sec.Fields[i].Apply = ApplyRestart
	}
	sec.Source = sec.ID
	if err := os.MkdirAll(r.dir, 0755); err != nil {
		return fmt.Errorf("settings: ensure schemas dir: %w", err)
	}
	raw, err := json.MarshalIndent(sec, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: marshal section: %w", err)
	}
	return atomicWrite(filepath.Join(r.dir, sec.ID+".json"), raw)
}

// Unregister removes a registered section by id; reports whether it existed.
func (r *Registry) Unregister(id string) (bool, error) {
	if !slugPattern.MatchString(id) {
		return false, fmt.Errorf("settings: invalid section id %q", id)
	}
	path := filepath.Join(r.dir, id+".json")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if err := os.Remove(path); err != nil {
		return false, fmt.Errorf("settings: remove %s: %w", id, err)
	}
	return true, nil
}

// validateSection enforces the registered-schema contract: a slug id, a name, at
// least one field, and every field namespaced (AGEZT_*), typed, and NOT shadowing
// a built-in env. reserved is the built-in env set.
func validateSection(sec Section, reserved map[string]bool) error {
	if !slugPattern.MatchString(sec.ID) {
		return fmt.Errorf("section id %q must be a slug (lowercase letters, digits, '-', '_')", sec.ID)
	}
	if strings.TrimSpace(sec.Name) == "" {
		return fmt.Errorf("section %q: name required", sec.ID)
	}
	if len(sec.Fields) == 0 {
		return fmt.Errorf("section %q: at least one field required", sec.ID)
	}
	seen := map[string]bool{}
	for _, f := range sec.Fields {
		if !envNamePattern.MatchString(f.Env) {
			return fmt.Errorf("field %q: env must match AGEZT_[A-Z0-9_]+ (registered settings are namespaced)", f.Env)
		}
		if reserved[f.Env] {
			return fmt.Errorf("field %q: shadows a built-in setting and is reserved", f.Env)
		}
		if seen[f.Env] {
			return fmt.Errorf("field %q: duplicated in section", f.Env)
		}
		seen[f.Env] = true
		if !validType(f.Type) {
			return fmt.Errorf("field %q: unknown type %q", f.Env, f.Type)
		}
		if f.Type == TypeSelect && len(f.Options) == 0 {
			return fmt.Errorf("field %q: select type requires options", f.Env)
		}
	}
	return nil
}

func validType(t FieldType) bool {
	switch t {
	case TypeText, TypePassword, TypeNumber, TypeBool, TypeCSV, TypeSelect:
		return true
	}
	return false
}
