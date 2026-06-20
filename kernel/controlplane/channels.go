// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/netguard"
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
	for _, m := range channel.Manifests() {
		sectionFields := fieldsBySection[m.ConfigSection]
		baseEnvs := make([]string, 0, len(sectionFields))
		for _, f := range sectionFields {
			baseEnvs = append(baseEnvs, f.Env)
		}
		// Accounts: the default instance ("") + every discovered "#label".
		accounts := make([]map[string]any, 0, 2)
		for _, label := range append([]string{""}, settings.AccountLabels(allKeys, baseEnvs)...) {
			accounts = append(accounts, map[string]any{
				"label":      label,
				"configured": configuredFor(m.RequiredEnv, label),
				"live":       channel.IsLiveInstance(channel.InstanceKey(m.Kind, label)),
				"fields":     fieldsFor(sectionFields, label),
			})
		}
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
			"fields":         fieldsFor(sectionFields, ""), // default-instance fields, back-compat
			"accounts":       accounts,
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
	// SSRF guard: require an http(s) URL, and (below) route the probe through
	// netguard so a request-supplied URL can't reach the cloud-metadata endpoint
	// or other link-local/multicast targets, even via a redirect. Loopback +
	// private ranges ARE allowed — the gateway is legitimately local/LAN.
	if u, err := url.Parse(base); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.url must be an http(s) gateway URL"})
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

	body, code, _, err := wgGatewayGET(statusURL, keyHeader, key, 1<<20)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": false, "error": "cannot reach gateway: " + err.Error()}})
		return
	}
	if code/100 != 2 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": false, "error": "gateway status " + http.StatusText(code), "http_status": code}})
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

// handleWhatsAppGatewayQR fetches the login QR from a self-hosted gateway and
// returns it as a data: URL, so the Channels wizard can render it inline — scan
// to log the gateway's WhatsApp session in without opening the gateway's own UI.
// Same stateless, SSRF-guarded probe as the status check.
func (s *Server) handleWhatsAppGatewayQR(conn net.Conn, req Request) {
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

	var qrURL, keyHeader string
	if backend == "evolution" {
		qrURL = base + "/instance/connect/" + session
		keyHeader = "apikey"
	} else {
		qrURL = base + "/api/" + session + "/auth/qr?format=image"
		keyHeader = "X-Api-Key"
	}

	body, code, ctype, err := wgGatewayGET(qrURL, keyHeader, key, 4<<20)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": false, "error": "cannot reach gateway: " + err.Error()}})
		return
	}
	if code/100 != 2 {
		// Often means already logged in (no QR) or wrong session.
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": false, "error": "no QR (gateway returned " + http.StatusText(code) + " — already logged in?)", "http_status": code}})
		return
	}

	dataURL := ""
	if strings.HasPrefix(ctype, "image/") {
		// WAHA returns the QR as a raw image.
		dataURL = "data:" + ctype + ";base64," + base64.StdEncoding.EncodeToString(body)
	} else {
		// Evolution returns JSON { base64: "<data url or raw base64>", code: "..." }.
		var j struct {
			Base64 string `json:"base64"`
		}
		_ = json.Unmarshal(body, &j)
		switch {
		case strings.HasPrefix(j.Base64, "data:"):
			dataURL = j.Base64
		case j.Base64 != "":
			dataURL = "data:image/png;base64," + j.Base64
		}
	}
	if dataURL == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": false, "error": "gateway did not return a QR image"}})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": true, "qr": dataURL}})
}

// wgGatewayGET issues an SSRF-guarded GET to a self-hosted gateway and returns
// the body, HTTP status, and content type. netguard screens every dial +
// redirect hop: loopback/private are allowed (the gateway is legitimately
// local/LAN), but link-local (incl. the 169.254.169.254 cloud-metadata
// endpoint), multicast, and unspecified targets are refused.
func wgGatewayGET(fullURL, keyHeader, key string, max int64) (body []byte, status int, contentType string, err error) {
	u, perr := url.Parse(fullURL)
	if perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, 0, "", &url.Error{Op: "parse", URL: fullURL, Err: perr}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, 0, "", err
	}
	if key != "" {
		hreq.Header.Set(keyHeader, key)
	}
	client := netguard.New(netguard.AllowLoopback(), netguard.AllowPrivate()).HTTPClient(10 * time.Second)
	resp, err := client.Do(hreq)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, max))
	return b, resp.StatusCode, resp.Header.Get("Content-Type"), nil
}

// handleProviderProbe checks whether an LLM provider endpoint is reachable by
// GETting its OpenAI-compatible /models list — the "connectivity status" behind
// a Connect button, so you can verify a keyless local runtime (Ollama, LM Studio)
// or a keyed endpoint is up before relying on it. Same SSRF-guarded probe as the
// gateway checks (loopback/private allowed; metadata/link-local blocked).
func (s *Server) handleProviderProbe(conn net.Conn, req Request) {
	base := strings.TrimRight(strings.TrimSpace(wgArg(req, "url")), "/")
	if base == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.url is required"})
		return
	}
	// OpenAI-compatible servers list models at <base>/models (base usually ends /v1).
	modelsURL := base + "/models"
	key := strings.TrimSpace(wgArg(req, "key"))
	body, code, _, err := wgGatewayGET(modelsURL, "Authorization", bearer(key), 1<<20)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": false, "error": "cannot reach endpoint: " + err.Error()}})
		return
	}
	// 2xx = reachable + authorized. 401/403 = reachable but needs/!valid key.
	reachable := code/100 == 2 || code == 401 || code == 403
	count := 0
	if code/100 == 2 {
		var parsed struct {
			Data []json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(body, &parsed)
		count = len(parsed.Data)
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"ok":          true,
		"reachable":   reachable,
		"authorized":  code/100 == 2,
		"http_status": code,
		"models":      count,
	}})
}

// bearer wraps a non-empty key as a Bearer value, else returns "".
func bearer(key string) string {
	if key == "" {
		return ""
	}
	return "Bearer " + key
}

// wgArg reads a string request arg, tolerating a missing/non-string value.
func wgArg(req Request, key string) string {
	v, _ := req.Args[key].(string)
	return v
}
