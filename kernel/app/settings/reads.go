// SPDX-License-Identifier: MIT
package settings

import (
	"context"
	core "github.com/agezt/agezt/kernel/settings"
)

type Section = core.Section
type Reader interface {
	SchemaSections() []Section
	PrepareValues() ValuesReader
}

// ValuesReader exposes secret presence and non-secret values through separate calls.
type ValuesReader interface {
	Sections() []Section
	EnvPinned(string) bool
	SecretSet(string) bool
	EnvValue(string) string
	StoredValue(string) (string, bool)
}
type Reads struct{ reader Reader }

func NewReads(reader Reader) *Reads { return &Reads{reader: reader} }

type SchemaInput struct{}
type ValuesInput struct{}

func (s *Reads) Schema(_ context.Context, _ SchemaInput) (SchemaOutput, error) {
	sections := cloneSections(s.reader.SchemaSections())
	return SchemaOutput{Sections: sections, ReloadBoundaries: core.ReloadBoundaries(sections)}, nil
}
func (s *Reads) Values(_ context.Context, _ ValuesInput) (ValuesOutput, error) {
	values := s.reader.PrepareValues()
	out := make([]ValueRow, 0, 32)
	for _, sec := range values.Sections() {
		for _, f := range sec.Fields {
			pinned := values.EnvPinned(f.Env)
			entry := ValueRow{Env: f.Env, Secret: f.Secret, EnvPinned: pinned}

			if f.Secret {
				// Presence only — the value never leaves the daemon.
				entry.Set = values.SecretSet(f.Env)
			} else {
				// Prefer the live env (covers env-pinned + our own injection),
				// fall back to the stored value.
				val := values.EnvValue(f.Env)
				if val == "" {
					val, _ = values.StoredValue(f.Env)
				}
				entry.Value = &val
				entry.Set = val != ""
			}
			out = append(out, entry)
		}
	}
	return ValuesOutput{Fields: out}, nil
}
