// SPDX-License-Identifier: MIT

// Control-plane channels: WhatsApp gateway status/QR proxy handlers + helpers.
// Code extracted from channels.go during the Day-113 god-file split.
// Public API unchanged.
package controlplane


import (
	"context"
	"io"
	"net"
	"strings"
	"time"

	"encoding/base64"
	"encoding/json"
	"github.com/agezt/agezt/kernel/netguard"
	"net/http"
	"net/url"
)

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
