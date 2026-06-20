// SPDX-License-Identifier: MIT

package controlplane

// Multi-account channel management: a channel kind can run several accounts at
// once (e.g. 10 email accounts, several Telegram bots). An account is a short
// LABEL; its field values are the base AGEZT_* env names suffixed "#<label>" —
// non-secret in the config store, secret in the vault — exactly the convention
// the credentials keyring uses. The unlabelled keys are the default account.
//
// These handlers mirror handleConfigSet (validate against the BASE field, route
// secret→vault / non-secret→store) but write to the suffixed key. Channels are
// ApplyRestart, so edits take effect on the next restart. Listing is served by
// handleChannelList's per-channel "accounts" array; testing reuses /api/send with
// a "kind#label" target.

import (
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/settings"
)

// sectionFieldFor returns the schema field for base env `name` IF it belongs to
// channel kind's config section — guarding against writing an arbitrary env via
// the account API.
func sectionFieldFor(baseDir, kind, name string) (settings.Field, bool) {
	m, ok := channel.LookupManifest(kind)
	if !ok {
		return settings.Field{}, false
	}
	reg := settings.NewRegistry(baseDir)
	for _, sec := range reg.Sections() {
		if sec.ID != m.ConfigSection {
			continue
		}
		for _, f := range sec.Fields {
			if f.Env == name {
				return f, true
			}
		}
	}
	return settings.Field{}, false
}

// handleChannelAccountSet writes one field value for a channel account instance.
// args: kind, label (""=default), name (base AGEZT_ env), value.
func (s *Server) handleChannelAccountSet(conn net.Conn, req Request) {
	kind, _ := req.Args["kind"].(string)
	label, _ := req.Args["label"].(string)
	name, _ := req.Args["name"].(string)
	value, _ := req.Args["value"].(string)
	kind = strings.TrimSpace(strings.ToLower(kind))
	label = strings.TrimSpace(label)
	name = strings.TrimSpace(name)
	if kind == "" || name == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.kind and args.name required"})
		return
	}
	if label != "" && !settings.ValidAccountLabel(label) {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "label must be a slug: lowercase letters/digits/-/_, max 32"})
		return
	}
	field, ok := sectionFieldFor(s.baseDir, kind, name)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: name + " is not a field of channel " + kind})
		return
	}
	if field.ReadOnly {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: name + " is read-only"})
		return
	}
	if err := settings.Validate(field, value); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	value = strings.TrimSpace(value)
	key := settings.SuffixEnv(name, label)

	if field.Secret {
		vault := creds.NewStore(s.baseDir)
		if err := vault.Load(); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "load vault: " + err.Error()})
			return
		}
		if value == "" {
			vault.Remove(key)
		} else {
			_ = vault.Set(key, value)
		}
		if err := vault.Save(); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save vault: " + err.Error()})
			return
		}
	} else {
		store := settings.NewStore(s.baseDir)
		if err := store.Load(); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "load config: " + err.Error()})
			return
		}
		if value == "" {
			store.Remove(key)
		} else {
			store.Set(key, value)
		}
		if err := store.Save(); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save config: " + err.Error()})
			return
		}
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"kind": kind, "label": label, "env": key, "saved": true, "applied": "restart",
	}})
}

// handleChannelAccountRemove deletes every stored field of a labelled account
// across the config store + vault. The default account ("") cannot be removed
// this way (clear its fields individually). args: kind, label.
func (s *Server) handleChannelAccountRemove(conn net.Conn, req Request) {
	kind, _ := req.Args["kind"].(string)
	label, _ := req.Args["label"].(string)
	kind = strings.TrimSpace(strings.ToLower(kind))
	label = strings.TrimSpace(label)
	if kind == "" || label == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.kind and a non-empty args.label required"})
		return
	}
	m, ok := channel.LookupManifest(kind)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown channel " + kind})
		return
	}
	baseEnvs := settings.SectionEnvs(m.ConfigSection)

	store := settings.NewStore(s.baseDir)
	_ = store.Load()
	vault := creds.NewStore(s.baseDir)
	_ = vault.Load()
	removed := 0
	for _, base := range baseEnvs {
		key := settings.SuffixEnv(base, label)
		if store.Remove(key) {
			removed++
		}
		if vault.Remove(key) {
			removed++
		}
	}
	if err := store.Save(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save config: " + err.Error()})
		return
	}
	if err := vault.Save(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save vault: " + err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"kind": kind, "label": label, "removed": removed, "applied": "restart",
	}})
}
