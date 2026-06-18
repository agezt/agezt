// SPDX-License-Identifier: MIT

package controlplane

import (
	"net"
	"os"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/settings"
)

// handleChannelList returns every registered channel manifest joined with its
// Config Center account fields and a configured flag — the data the Channels
// wizard renders. Read-only. Secret fields report presence only (the value
// never leaves the daemon); non-secret fields carry their current value. A
// channel is "configured" when all its required env vars are set (store, vault,
// or a real-env pin).
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

	// isSet reports whether an env var has a value anywhere (env > vault > store).
	isSet := func(f settings.Field) (set bool, value string) {
		if f.Secret {
			return os.Getenv(f.Env) != "" || vault.Has(f.Env), ""
		}
		val := os.Getenv(f.Env)
		if val == "" {
			val, _ = store.Get(f.Env)
		}
		return val != "", val
	}

	rows := make([]map[string]any, 0, len(channel.Manifests()))
	for _, m := range channel.Manifests() {
		fields := make([]map[string]any, 0)
		for _, f := range fieldsBySection[m.ConfigSection] {
			set, value := isSet(f)
			fld := map[string]any{
				"env":        f.Env,
				"label":      f.Label,
				"secret":     f.Secret,
				"required":   f.Required,
				"help":       f.Help,
				"set":        set,
				"env_pinned": s.configEnvPinned[f.Env],
			}
			if !f.Secret {
				fld["value"] = value
			}
			fields = append(fields, fld)
		}
		configured := true
		for _, env := range m.RequiredEnv {
			present := os.Getenv(env) != "" || vault.Has(env)
			if !present {
				if v, _ := store.Get(env); v == "" {
					configured = false
					break
				}
			}
		}
		rows = append(rows, map[string]any{
			"kind":           m.Kind,
			"display":        m.Display,
			"description":    m.Description,
			"transport":      m.Transport,
			"duplex":         m.Duplex,
			"config_section": m.ConfigSection,
			"docs_url":       m.DocsURL,
			"configured":     configured,
			"fields":         fields,
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"channels": rows,
		"count":    len(rows),
	}})
}
