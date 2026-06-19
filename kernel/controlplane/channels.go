// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

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
			"live":           channel.IsLive(m.Kind),
			"fields":         fields,
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"channels": rows,
		"count":    len(rows),
	}})
}

// handleWhatsAppGatewayStatus probes a self-hosted WhatsApp gateway (WAHA or
// Evolution) and reports whether its WhatsApp session is logged in — so the
// Channels wizard can tell the operator "connected" vs "scan the QR" without
// leaving the console. Stateless: the gateway URL/backend/session/key come from
// the request (the wizard's current form), so it works before a restart. The
// URL is operator-supplied (their own gateway), so this is a trusted probe.
func (s *Server) handleWhatsAppGatewayStatus(conn net.Conn, req Request) {
	base := strings.TrimRight(strings.TrimSpace(wgArg(req, "url")), "/")
	if base == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.url (gateway URL) is required"})
		return
	}
	backend := strings.ToLower(strings.TrimSpace(wgArg(req, "backend")))
	session := strings.TrimSpace(wgArg(req, "session"))
	if session == "" {
		session = "default"
	}
	key := strings.TrimSpace(wgArg(req, "key"))

	var statusURL, keyHeader string
	if backend == "evolution" {
		statusURL = base + "/instance/connectionState/" + session
		keyHeader = "apikey"
	} else {
		statusURL = base + "/api/sessions/" + session
		keyHeader = "X-Api-Key"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if key != "" {
		hreq.Header.Set(keyHeader, key)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(hreq)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": false, "error": "cannot reach gateway: " + err.Error()}})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": false, "error": "gateway status " + http.StatusText(resp.StatusCode), "http_status": resp.StatusCode}})
		return
	}
	// Accept both shapes: WAHA {status:"WORKING"} and Evolution {instance:{state:"open"}}.
	var parsed struct {
		Status   string `json:"status"`
		State    string `json:"state"`
		Instance struct {
			State string `json:"state"`
		} `json:"instance"`
	}
	_ = json.Unmarshal(body, &parsed)
	status := parsed.Status
	if status == "" {
		status = parsed.State
	}
	if status == "" {
		status = parsed.Instance.State
	}
	connected := status == "WORKING" || strings.EqualFold(status, "open")
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"ok":        true,
		"connected": connected,
		"status":    status,
	}})
}

// wgArg reads a string request arg, tolerating a missing/non-string value.
func wgArg(req Request, key string) string {
	v, _ := req.Args[key].(string)
	return v
}
