// SPDX-License-Identifier: MIT

package controlplane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestGatewayPlatformPreservesNativeProviderAndWhatsAppResults(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			if r.Header.Get("Authorization") != "Bearer fixture" {
				t.Error("provider key lost")
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"one"},{"id":"two"}]}`))
		case "/api/sessions/default":
			if r.Header.Get("X-Api-Key") != "fixture" {
				t.Error("gateway key lost")
			}
			_, _ = w.Write([]byte(`{"status":"WORKING"}`))
		case "/api/default/auth/qr":
			if r.URL.Query().Get("format") != "image" {
				t.Error("QR query lost")
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png-fixture"))
		default:
			t.Errorf("unexpected path %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer endpoint.Close()
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, dir)
	s.token = "primary"
	for _, cmd := range []string{CmdProviderProbe, CmdWhatsAppGatewayStatus, CmdWhatsAppGatewayQR} {
		base := endpoint.URL
		if cmd == CmdProviderProbe {
			base += "/v1/"
		}
		responses := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"url": base, "key": " fixture "}})
		last := responses[len(responses)-1]
		if last.Type != RespResult || last.Result["ok"] != true {
			t.Fatalf("%s = %+v", cmd, last)
		}
		switch cmd {
		case CmdProviderProbe:
			if last.Result["models"] != float64(2) || last.Result["reachable"] != true || last.Result["authorized"] != true || last.Result["http_status"] != float64(200) {
				t.Fatalf("probe = %v", last.Result)
			}
		case CmdWhatsAppGatewayStatus:
			if last.Result["connected"] != true || last.Result["status"] != "WORKING" {
				t.Fatalf("gateway status = %v", last.Result)
			}
		case CmdWhatsAppGatewayQR:
			if qr, _ := last.Result["qr"].(string); !strings.HasPrefix(qr, "data:image/png;base64,") {
				t.Fatalf("QR = %v", last.Result)
			}
		}
	}
}
