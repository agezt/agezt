// SPDX-License-Identifier: MIT

// Settings schema: FieldType/Apply/Field/Section types + SourceBuiltin constant + Schema().
// Code extracted from schema.go during the Day-62 god-file split. Public API unchanged.
package settings






// FieldType drives how the UI renders a field and how the server validates it.
type FieldType string

const (
	TypeText     FieldType = "text"
	TypePassword FieldType = "password" // secret; rendered masked, stored in the vault
	TypeNumber   FieldType = "number"
	TypeBool     FieldType = "bool"
	TypeCSV      FieldType = "csv" // comma-separated list (allowlists, recipients)
	TypeSelect   FieldType = "select"
)

// Apply says whether a change takes effect immediately or needs a restart. Only
// provider/model/catalog hot-reload today (via provider_reload); channels and
// interfaces are read once at startup.
type Apply string

const (
	ApplyLive    Apply = "live"
	ApplyRestart Apply = "restart"
)

// Field is one editable setting, keyed by its exact AGEZT_* env-var name.
type Field struct {
	Env      string    `json:"env"`
	Label    string    `json:"label"`
	Type     FieldType `json:"type"`
	Secret   bool      `json:"secret"` // true → stored in the vault, never echoed back
	Required bool      `json:"required"`
	Help     string    `json:"help,omitempty"`
	Apply    Apply     `json:"apply"`
	Options  []string  `json:"options,omitempty"` // for TypeSelect
	// ReadOnly: shown in the Config Center but NOT editable there (system-managed).
	// The server rejects any config_set for it; the UI renders it read-only.
	ReadOnly bool `json:"read_only,omitempty"`
	// Locked: the value may be changed but never CLEARED/removed ("silinemez").
	// The server rejects a config_set with an empty value; the UI hides Clear.
	Locked bool `json:"locked,omitempty"`
}

// Section groups related fields for the Config Center UI. Source records where
// the section came from — "builtin" for the compiled-in core config, or the
// registered schema's id for a skill/plugin-contributed section (see registry.go).
type Section struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Help   string `json:"help,omitempty"`
	Source string `json:"source,omitempty"`
	// Locked: a system-approved section that cannot be unregistered through the
	// normal path (config_schema_unregister / the `config` tool) — only with an
	// explicit operator force, or by deleting the file. Built-in sections are
	// always permanent regardless of this flag.
	Locked bool    `json:"locked,omitempty"`
	Fields []Field `json:"fields"`
}

// SourceBuiltin marks the compiled-in core configuration sections.
const SourceBuiltin = "builtin"

// Schema returns the built-in editable configuration surface. Kept for
// back-compat and as the seed for the Registry (registry.go), which merges
// these compiled-in sections with on-disk skill/plugin-registered ones.
func Schema() []Section {
	return builtinSections()
}