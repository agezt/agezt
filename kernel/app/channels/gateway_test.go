// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
)

func TestChannelGatewayPortURLHeaderBoundsAndLegacyContext(t *testing.T) {
	for _, qr := range []bool{false, true} {
		for _, backend := range []string{"WAHA", " EVOLUTION ", "unknown"} {
			for _, session := range []string{"", " owned session "} {
				calls := 0
				service := NewGateway(func(full, header, key string, max int64) ([]byte, int, string, error) {
					calls++
					name := "owned session"
					if session == "" {
						name = "default"
					}
					path, wantHeader, bound := "/api/sessions/"+name, "X-Api-Key", int64(1<<20)
					if qr {
						path, bound = "/api/"+name+"/auth/qr?format=image", 4<<20
					}
					if backend == " EVOLUTION " {
						wantHeader = "apikey"
						path = "/instance/connectionState/" + name
						if qr {
							path = "/instance/connect/" + name
						}
					}
					if full != "http://owned.example/base"+path || header != wantHeader || key != "owned secret" || max != bound {
						t.Fatalf("port %q %q %q %d", full, header, key, max)
					}
					if qr {
						return []byte("owned-png"), 200, "image/png", nil
					}
					return []byte(`{"status":"WORKING"}`), 200, "application/json", nil
				})
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				input := GatewayInput{URL: " http://owned.example/base/// ", Backend: backend, Session: session, Key: " owned secret "}
				var result GatewayOutput
				var err error
				if qr {
					result, err = service.QR(ctx, input)
				} else {
					result, err = service.Status(ctx, input)
				}
				want := GatewayOutput{"ok": true, "status": "WORKING", "connected": true}
				if qr {
					want = GatewayOutput{"ok": true, "qr": "data:image/png;base64,b3duZWQtcG5n"}
				}
				if err != nil || calls != 1 || !reflect.DeepEqual(result, want) {
					t.Fatal(result, err, calls)
				}
			}
		}
	}
}

func TestChannelGatewayValidationPrecedenceAndUnavailableErrors(t *testing.T) {
	calls := 0
	service := NewGateway(func(string, string, string, int64) ([]byte, int, string, error) {
		calls++
		return nil, 0, "", errors.New("owned failure")
	})
	for _, qr := range []bool{false, true} {
		for _, input := range []string{"", " /// "} {
			var err error
			if qr {
				_, err = service.QR(context.Background(), GatewayInput{URL: input})
			} else {
				_, err = service.Status(context.Background(), GatewayInput{URL: input})
			}
			if err == nil || err.Error() != "args.url (gateway URL) is required" || calls != 0 {
				t.Fatal(err, calls)
			}
		}
	}
	for _, invalid := range []string{"file:///owned", "https://", ":bad"} {
		_, err := service.Status(context.Background(), GatewayInput{URL: invalid})
		if err == nil || err.Error() != "args.url must be an http(s) gateway URL" || calls != 0 {
			t.Fatal(err, calls)
		}
	}
	// QR delegates URL validation to the selected platform port, preserving its
	// legacy result/error boundary instead of adopting Status's early rejection.
	for _, qr := range []bool{false, true} {
		var result GatewayOutput
		var err error
		input := GatewayInput{URL: "http://owned.example"}
		if qr {
			input.URL = "file:///owned"
			result, err = service.QR(context.Background(), input)
		} else {
			result, err = service.Status(context.Background(), input)
		}
		if err != nil || !reflect.DeepEqual(result, GatewayOutput{"ok": false, "error": "cannot reach gateway: owned failure"}) {
			t.Fatal(result, err)
		}
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestChannelGatewayStatusPresentationPrecedenceAndMalformedResponses(t *testing.T) {
	for _, tc := range []struct {
		body, status string
		connected    bool
	}{
		{`{"status":"WORKING","state":"closed","instance":{"state":"bad"}}`, "WORKING", true},
		{`{"status":"working","state":"open"}`, "working", false},
		{`{"state":"OPEN","instance":{"state":"closed"}}`, "OPEN", true},
		{`{"instance":{"state":"open"}}`, "open", true},
		{`{"status":"closed"}`, "closed", false},
		{`bad-json`, "", false},
		{`{"status":5,"state":"open"}`, "open", true},
	} {
		service := NewGateway(func(string, string, string, int64) ([]byte, int, string, error) {
			return []byte(tc.body), 200, "application/json", nil
		})
		result, err := service.Status(context.Background(), GatewayInput{URL: "https://owned.example"})
		if err != nil || !reflect.DeepEqual(result, GatewayOutput{"ok": true, "status": tc.status, "connected": tc.connected}) {
			t.Fatal(tc, result, err)
		}
	}
	for _, code := range []int{204, 299, 400, 503, 599} {
		service := NewGateway(func(string, string, string, int64) ([]byte, int, string, error) { return nil, code, "", nil })
		result, err := service.Status(context.Background(), GatewayInput{URL: "https://owned.example"})
		if err != nil {
			t.Fatal(err)
		}
		if code/100 == 2 {
			if result["ok"] != true || result["status"] != "" || result["connected"] != false {
				t.Fatal(result)
			}
		} else {
			if !reflect.DeepEqual(result, GatewayOutput{"ok": false, "http_status": code, "error": "gateway status " + http.StatusText(code)}) {
				t.Fatal(result)
			}
		}
	}
}

func TestChannelGatewayQRRawImageJSONAndHTTPFailure(t *testing.T) {
	for _, tc := range []struct {
		body, ctype string
		code        int
		want        GatewayOutput
	}{
		{"png", "image/png", 200, GatewayOutput{"ok": true, "qr": "data:image/png;base64,cG5n"}},
		{"", "image/jpeg; charset=binary", 200, GatewayOutput{"ok": true, "qr": "data:image/jpeg; charset=binary;base64,"}},
		{`{"base64":"data:image/svg+xml;base64,owned"}`, "application/json", 200, GatewayOutput{"ok": true, "qr": "data:image/svg+xml;base64,owned"}},
		{`{"base64":"raw"}`, "application/json", 200, GatewayOutput{"ok": true, "qr": "data:image/png;base64,raw"}},
		{`{"base64":""}`, "application/json", 200, GatewayOutput{"ok": false, "error": "gateway did not return a QR image"}},
		{"bad", "Image/png", 200, GatewayOutput{"ok": false, "error": "gateway did not return a QR image"}},
		{"bad", "image/png", 403, GatewayOutput{"ok": false, "error": "no QR (gateway returned Forbidden — already logged in?)", "http_status": 403}},
	} {
		service := NewGateway(func(string, string, string, int64) ([]byte, int, string, error) {
			return []byte(tc.body), tc.code, tc.ctype, nil
		})
		result, err := service.QR(context.Background(), GatewayInput{URL: "https://owned.example"})
		if err != nil || !reflect.DeepEqual(result, tc.want) {
			t.Fatal(tc, result, err)
		}
	}
}
