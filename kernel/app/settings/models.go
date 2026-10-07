// SPDX-License-Identifier: MIT
package settings

import core "github.com/agezt/agezt/kernel/settings"

type SchemaOutput struct {
	Sections         []Section             `json:"sections"`
	ReloadBoundaries []core.ReloadBoundary `json:"reload_boundaries"`
}
type ValueRow struct {
	Env       string  `json:"env"`
	Secret    bool    `json:"secret"`
	EnvPinned bool    `json:"env_pinned"`
	Set       bool    `json:"set"`
	Value     *string `json:"value,omitempty"`
}
type ValuesOutput struct {
	Fields []ValueRow `json:"fields"`
}
type SetOutput struct {
	Env         string  `json:"env"`
	Saved       bool    `json:"saved"`
	Applied     string  `json:"applied"`
	EnvPinned   bool    `json:"env_pinned,omitempty"`
	ReloadError *string `json:"reload_error,omitempty"`
}
type RegisterOutput struct {
	ID         string `json:"id"`
	Registered bool   `json:"registered"`
	Applied    string `json:"applied"`
}
type UnregisterOutput struct {
	ID      string `json:"id"`
	Removed bool   `json:"removed"`
}

func cloneSections(sections []Section) []Section {
	if sections == nil {
		return nil
	}
	out := make([]Section, len(sections))
	copy(out, sections)
	for i := range out {
		if sections[i].Fields != nil {
			out[i].Fields = make([]core.Field, len(sections[i].Fields))
			copy(out[i].Fields, sections[i].Fields)
		}
		for j := range out[i].Fields {
			options := sections[i].Fields[j].Options
			if options != nil {
				out[i].Fields[j].Options = make([]string, len(options))
				copy(out[i].Fields[j].Options, options)
			}
		}
	}
	return out
}
