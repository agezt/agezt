// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GatewayGET is the bounded selected HTTP port. Its legacy background context
// policy is retained by this move; context refinement is a separate change.
type GatewayGET func(string, string, string, int64) ([]byte, int, string, error)
type Gateway struct{ get GatewayGET }

func NewGateway(get GatewayGET) *Gateway { return &Gateway{get: get} }

type GatewayInput struct{ URL, Backend, Session, Key string }
type GatewayOutput = map[string]any

func (s *Gateway) Status(_ context.Context, in GatewayInput) (GatewayOutput, error) {
	base := strings.TrimRight(strings.TrimSpace(in.URL), "/")
	if base == "" {
		return nil, fmt.Errorf("args.url (gateway URL) is required")
	}
	// SSRF guard: require an http(s) URL, and (below) route the probe through
	// netguard so a request-supplied URL can't reach the cloud-metadata endpoint
	// or other link-local/multicast targets, even via a redirect. Loopback +
	// private ranges ARE allowed — the gateway is legitimately local/LAN.
	if u, err := url.Parse(base); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("args.url must be an http(s) gateway URL")
	}
	backend := strings.ToLower(strings.TrimSpace(in.Backend))
	session := strings.TrimSpace(in.Session)
	if session == "" {
		session = "default"
	}
	key := strings.TrimSpace(in.Key)

	var statusURL, keyHeader string
	if backend == "evolution" {
		statusURL = base + "/instance/connectionState/" + session
		keyHeader = "apikey"
	} else {
		statusURL = base + "/api/sessions/" + session
		keyHeader = "X-Api-Key"
	}

	body, code, _, err := s.get(statusURL, keyHeader, key, 1<<20)
	if err != nil {
		return map[string]any{"ok": false, "error": "cannot reach gateway: " + err.Error()}, nil
	}
	if code/100 != 2 {
		return map[string]any{"ok": false, "error": "gateway status " + http.StatusText(code), "http_status": code}, nil
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
	return map[string]any{
		"ok":        true,
		"connected": connected,
		"status":    status,
	}, nil
}

// handleWhatsAppGatewayQR fetches the login QR from a self-hosted gateway and
// returns it as a data: URL, so the Channels wizard can render it inline — scan
// to log the gateway's WhatsApp session in without opening the gateway's own UI.
// Same stateless, SSRF-guarded probe as the status check.
func (s *Gateway) QR(_ context.Context, in GatewayInput) (GatewayOutput, error) {
	base := strings.TrimRight(strings.TrimSpace(in.URL), "/")
	if base == "" {
		return nil, fmt.Errorf("args.url (gateway URL) is required")
	}
	backend := strings.ToLower(strings.TrimSpace(in.Backend))
	session := strings.TrimSpace(in.Session)
	if session == "" {
		session = "default"
	}
	key := strings.TrimSpace(in.Key)

	var qrURL, keyHeader string
	if backend == "evolution" {
		qrURL = base + "/instance/connect/" + session
		keyHeader = "apikey"
	} else {
		qrURL = base + "/api/" + session + "/auth/qr?format=image"
		keyHeader = "X-Api-Key"
	}

	body, code, ctype, err := s.get(qrURL, keyHeader, key, 4<<20)
	if err != nil {
		return map[string]any{"ok": false, "error": "cannot reach gateway: " + err.Error()}, nil
	}
	if code/100 != 2 {
		// Often means already logged in (no QR) or wrong session.
		return map[string]any{"ok": false, "error": "no QR (gateway returned " + http.StatusText(code) + " — already logged in?)", "http_status": code}, nil
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
		return map[string]any{"ok": false, "error": "gateway did not return a QR image"}, nil
	}
	return map[string]any{"ok": true, "qr": dataURL}, nil
}
