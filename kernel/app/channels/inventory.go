// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/settings"
)

type InventoryReader interface {
	Prepare() InventoryValues
	Manifests() []channel.Manifest
	IsLive(string) bool
	IsLiveInstance(string) bool
}
type InventoryValues interface {
	Sections() []settings.Section
	EnvValue(string) string
	SecretSet(string) bool
	StoredValue(string) (string, bool)
	EnvPinned(string) bool
	Names() []string
}
type Inventory struct{ reader InventoryReader }

func NewInventory(reader InventoryReader) *Inventory { return &Inventory{reader: reader} }

type ListInput struct{}

func (s *Inventory) List(_ context.Context, _ ListInput) (ListOutput, error) {
	values := s.reader.Prepare()
	// Index section fields by id for quick lookup.
	fieldsBySection := map[string][]settings.Field{}
	for _, sec := range values.Sections() {
		fieldsBySection[sec.ID] = sec.Fields
	}

	// isSetKey reports whether a (possibly "#label"-suffixed) env key has a value
	// anywhere (env > vault > store). Secrets report presence only.
	isSetKey := func(f settings.Field, key string) (set bool, value string) {
		if f.Secret {
			return values.EnvValue(key) != "" || values.SecretSet(key), ""
		}
		val := values.EnvValue(key)
		if val == "" {
			val, _ = values.StoredValue(key)
		}
		return val != "", val
	}
	// fieldsFor builds the per-field presence/value list for one account label.
	fieldsFor := func(sectionFields []settings.Field, label string) []ChannelField {
		out := make([]ChannelField, 0, len(sectionFields))
		for _, f := range sectionFields {
			key := settings.SuffixEnv(f.Env, label)
			set, value := isSetKey(f, key)
			fld := ChannelField{Env: f.Env, Label: f.Label, Secret: f.Secret, Required: f.Required, Help: f.Help, Set: set, EnvPinned: label == "" && values.EnvPinned(f.Env)}
			if !f.Secret {
				fld.Value = &value
			}
			out = append(out, fld)
		}
		return out
	}
	// configuredFor reports whether all required envs are present for a label.
	configuredFor := func(required []string, label string) bool {
		for _, env := range required {
			key := settings.SuffixEnv(env, label)
			if values.EnvValue(key) == "" && !values.SecretSet(key) {
				if v, _ := values.StoredValue(key); v == "" {
					return false
				}
			}
		}
		return true
	}

	allKeys := values.Names()
	rows := make([]ChannelRow, 0, len(s.reader.Manifests()))
	probeMatrix := ProbeMatrix{}
	mediaMatrix := MediaMatrix{}
	for _, m := range s.reader.Manifests() {
		sectionFields := fieldsBySection[m.ConfigSection]
		baseEnvs := make([]string, 0, len(sectionFields))
		for _, f := range sectionFields {
			baseEnvs = append(baseEnvs, f.Env)
		}
		// Accounts: the default instance ("") + every discovered "#label".
		accounts := make([]ChannelAccount, 0, 2)
		for _, label := range append([]string{""}, settings.AccountLabels(allKeys, baseEnvs)...) {
			configured := configuredFor(m.RequiredEnv, label)
			live := s.reader.IsLiveInstance(channel.InstanceKey(m.Kind, label))
			accounts = append(accounts, ChannelAccount{Label: label, Configured: configured, Live: live, Probe: channelAccountProbe(m, configured, live), Fields: fieldsFor(sectionFields, label)})
		}
		probe := channelProbe(m, accounts)
		addChannelProbeTotals(&probeMatrix, probe)
		addChannelMediaTotals(&mediaMatrix, m.Media)
		rows = append(rows, ChannelRow{Kind: m.Kind, Display: m.Display, Description: m.Description, Transport: m.Transport, Duplex: m.Duplex, Media: m.Media, SetupSteps: m.SetupSteps, ConnectMethod: m.ConnectMethod, ConfigSection: m.ConfigSection, DocsURL: m.DocsURL, Configured: configuredFor(m.RequiredEnv, ""), Live: s.reader.IsLive(m.Kind), Probe: probe, Fields: fieldsFor(sectionFields, ""), Accounts: accounts})
	}
	return ListOutput{Channels: rows, Count: len(rows), ProbeMatrix: probeMatrix, MediaMatrix: mediaMatrix}, nil
}
