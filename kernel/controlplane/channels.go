// SPDX-License-Identifier: MIT
//
// kernel/controlplane channel-list handler (handleChannelList).
// Extracted from channels.go during Day 211 god-file refactor (#82).
// Public API unchanged.
package controlplane

import (
	"net"
	"os"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/settings"
)

func (s *Server) handleChannelList(conn net.Conn, req Request) {
	store := settings.NewStore(s.baseDir)
	_ = store.Load()
	vault := creds.NewStore(s.baseDir)
	_ = vault.Load()
	reg := settings.NewRegistry(s.baseDir)

	// Index section fields by id for quick lookup.
	fieldsBySection := map[string][]settings.Field{}
	for _, sec := range reg.Sections() {
		fieldsBySection[sec.ID] = sec.Fields
	}

	// isSetKey reports whether a (possibly "#label"-suffixed) env key has a value
	// anywhere (env > vault > store). Secrets report presence only.
	isSetKey := func(f settings.Field, key string) (set bool, value string) {
		if f.Secret {
			return os.Getenv(key) != "" || vault.Has(key), ""
		}
		val := os.Getenv(key)
		if val == "" {
			val, _ = store.Get(key)
		}
		return val != "", val
	}
	// fieldsFor builds the per-field presence/value list for one account label.
	fieldsFor := func(sectionFields []settings.Field, label string) []map[string]any {
		out := make([]map[string]any, 0, len(sectionFields))
		for _, f := range sectionFields {
			key := settings.SuffixEnv(f.Env, label)
			set, value := isSetKey(f, key)
			fld := map[string]any{
				"env":        f.Env, // base env; the label addresses the instance
				"label":      f.Label,
				"secret":     f.Secret,
				"required":   f.Required,
				"help":       f.Help,
				"set":        set,
				"env_pinned": label == "" && s.configEnvPinned[f.Env],
			}
			if !f.Secret {
				fld["value"] = value
			}
			out = append(out, fld)
		}
		return out
	}
	// configuredFor reports whether all required envs are present for a label.
	configuredFor := func(required []string, label string) bool {
		for _, env := range required {
			key := settings.SuffixEnv(env, label)
			if os.Getenv(key) == "" && !vault.Has(key) {
				if v, _ := store.Get(key); v == "" {
					return false
				}
			}
		}
		return true
	}

	allKeys := append(store.Names(), vault.Names()...)
	rows := make([]map[string]any, 0, len(channel.Manifests()))
	probeMatrix := map[string]any{
		"total":           0,
		"configured":      0,
		"live":            0,
		"roundtrip_ready": 0,
		"restart_needed":  0,
		"needs_setup":     0,
	}
	mediaMatrix := map[string]any{
		"image_in":  0,
		"image_out": 0,
		"voice_in":  0,
		"voice_out": 0,
	}
	for _, m := range channel.Manifests() {
		sectionFields := fieldsBySection[m.ConfigSection]
		baseEnvs := make([]string, 0, len(sectionFields))
		for _, f := range sectionFields {
			baseEnvs = append(baseEnvs, f.Env)
		}
		// Accounts: the default instance ("") + every discovered "#label".
		accounts := make([]map[string]any, 0, 2)
		for _, label := range append([]string{""}, settings.AccountLabels(allKeys, baseEnvs)...) {
			configured := configuredFor(m.RequiredEnv, label)
			live := channel.IsLiveInstance(channel.InstanceKey(m.Kind, label))
			accounts = append(accounts, map[string]any{
				"label":      label,
				"configured": configured,
				"live":       live,
				"probe":      channelAccountProbe(m, configured, live),
				"fields":     fieldsFor(sectionFields, label),
			})
		}
		probe := channelProbe(m, accounts)
		addChannelProbeTotals(probeMatrix, probe)
		addChannelMediaTotals(mediaMatrix, m.Media)
		rows = append(rows, map[string]any{
			"kind":           m.Kind,
			"display":        m.Display,
			"description":    m.Description,
			"transport":      m.Transport,
			"duplex":         m.Duplex,
			"media":          m.Media,
			"setup_steps":    m.SetupSteps,
			"connect_method": m.ConnectMethod,
			"config_section": m.ConfigSection,
			"docs_url":       m.DocsURL,
			"configured":     configuredFor(m.RequiredEnv, ""), // default-instance, back-compat
			"live":           channel.IsLive(m.Kind),
			"probe":          probe,
			"fields":         fieldsFor(sectionFields, ""), // default-instance fields, back-compat
			"accounts":       accounts,
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"channels":     rows,
		"count":        len(rows),
		"probe_matrix": probeMatrix,
		"media_matrix": mediaMatrix,
	}})
}
