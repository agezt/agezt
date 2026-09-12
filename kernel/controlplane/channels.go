// SPDX-License-Identifier: MIT

// Control-plane channels: list handler + probe helpers + provider probe.
// Code extracted from channels.go during the Day-113 god-file split.
// Public API unchanged.
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

func channelAccountProbe(m channel.Manifest, configured, live bool) map[string]any {
	status := "needs_setup"
	note := "required channel settings are missing"
	switch {
	case live:
		status = "ready"
		if m.Duplex {
			note = "live two-way account can receive and send a test reply"
		} else {
			note = "live outbound account can send a test message"
		}
	case configured:
		status = "restart_required"
		note = "configured but not live in this daemon process"
	}
	return map[string]any{
		"configured":       configured,
		"live":             live,
		"roundtrip_status": status,
		"roundtrip_ready":  live,
		"mode":             channelProbeMode(m),
		"note":             note,
	}
}

func channelProbe(m channel.Manifest, accounts []map[string]any) map[string]any {
	configuredAccounts := 0
	liveAccounts := 0
	for _, account := range accounts {
		if b, _ := account["configured"].(bool); b {
			configuredAccounts++
		}
		if b, _ := account["live"].(bool); b {
			liveAccounts++
		}
	}
	status := "needs_setup"
	switch {
	case liveAccounts > 0:
		status = "ready"
	case configuredAccounts > 0:
		status = "restart_required"
	}
	return map[string]any{
		"accounts":            len(accounts),
		"configured_accounts": configuredAccounts,
		"live_accounts":       liveAccounts,
		"roundtrip_status":    status,
		"roundtrip_ready":     liveAccounts > 0,
		"mode":                channelProbeMode(m),
	}
}

func channelProbeMode(m channel.Manifest) string {
	if m.Duplex {
		return "two_way"
	}
	return "outbound"
}

func addChannelProbeTotals(matrix map[string]any, probe map[string]any) {
	incr := func(key string) {
		n, _ := matrix[key].(int)
		matrix[key] = n + 1
	}
	incr("total")
	if n, _ := probe["configured_accounts"].(int); n > 0 {
		incr("configured")
	}
	if n, _ := probe["live_accounts"].(int); n > 0 {
		incr("live")
	}
	switch probe["roundtrip_status"] {
	case "ready":
		incr("roundtrip_ready")
	case "restart_required":
		incr("restart_needed")
	default:
		incr("needs_setup")
	}
}

func addChannelMediaTotals(matrix map[string]any, media channel.MediaCaps) {
	incr := func(key string) {
		n, _ := matrix[key].(int)
		matrix[key] = n + 1
	}
	if media.ImageIn {
		incr("image_in")
	}
	if media.ImageOut {
		incr("image_out")
	}
	if media.VoiceIn {
		incr("voice_in")
	}
	if media.VoiceOut {
		incr("voice_out")
	}
}

// handleWhatsAppGatewayStatus probes a self-hosted WhatsApp gateway (WAHA or
// Evolution) and reports whether its WhatsApp session is logged in — so the
// Channels wizard can tell the operator "connected" vs "scan the QR" without
// leaving the console. Stateless: the gateway URL/backend/session/key come from
// the request (the wizard's current form), so it works before a restart. The
// URL is operator-supplied (their own gateway), so this is a trusted probe.
