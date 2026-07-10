// SPDX-License-Identifier: MIT

package wecom

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/journal"
)

func newBus(t *testing.T) *bus.Bus {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	return b
}

// signedInbound builds a signed, encrypted POST body + query params for a
// WeCom inbound message, mirroring the WXBizMsgCrypt wire format.
func signedInbound(t *testing.T, c *Channel, token, inner string) (body string, sig, ts, nonce string) {
	t.Helper()
	enc, err := encryptTestPayload(c, inner, make([]byte, 16), "corpid")
	if err != nil {
		t.Fatal(err)
	}
	ts, nonce = "1000", "abc"
	sig = signature(token, ts, nonce, enc)
	xmlBody, _ := xml.Marshal(struct {
		XMLName xml.Name `xml:"xml"`
		Encrypt string   `xml:"Encrypt"`
	}{Encrypt: enc})
	return string(xmlBody), sig, ts, nonce
}

func TestName(t *testing.T) {
	if got := New(Config{}).Name(); got != "wecom" {
		t.Fatalf("Name = %q, want wecom", got)
	}
}

func TestNewDefaultsAndBadAESKey(t *testing.T) {
	c := New(Config{})
	if c.path != DefaultPath || c.apiBase != defaultAPIBase || c.client == nil {
		t.Fatalf("defaults not applied: %+v", c)
	}
	// Invalid base64 AES key leaves aesKey nil (channel stays outbound-only).
	c2 := New(Config{AESKey: "!!!not-base64!!!"})
	if c2.aesKey != nil {
		t.Fatal("bad AES key should leave aesKey nil")
	}
	// Overrides retained; trailing slash trimmed.
	c3 := New(Config{Path: "/wc", APIBase: "https://x.example/", HTTPClient: &http.Client{}})
	if c3.path != "/wc" || c3.apiBase != "https://x.example" {
		t.Fatalf("overrides not applied: path=%q base=%q", c3.path, c3.apiBase)
	}
}

func TestStartNoAddrBlocksUntilCancel(t *testing.T) {
	c := New(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return")
	}
}

func TestStartServesThenShutsDown(t *testing.T) {
	c := New(Config{Addr: "127.0.0.1:39461", AESKey: testAESKey(), Token: "tok"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	time.Sleep(150 * time.Millisecond)
	// A malformed GET (bad signature) still proves the server is up.
	resp, err := http.Get("http://127.0.0.1:39461" + DefaultPath + "?msg_signature=x&timestamp=1&nonce=n&echostr=e")
	if err == nil {
		resp.Body.Close()
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestHandlerGetVerification(t *testing.T) {
	token := "tok"
	c := New(Config{AESKey: testAESKey(), Token: token})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()

	// Build an encrypted echostr and its signature.
	echo, err := encryptTestPayload(c, "hello-echo", make([]byte, 16), "corpid")
	if err != nil {
		t.Fatal(err)
	}
	ts, nonce := "100", "n1"
	sig := signature(token, ts, nonce, echo)
	q := url.Values{"msg_signature": {sig}, "timestamp": {ts}, "nonce": {nonce}, "echostr": {echo}}
	resp, err := http.Get(srv.URL + DefaultPath + "?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "hello-echo" {
		t.Fatalf("echo = %q, want decrypted plaintext", b)
	}
}

func TestHandlerGetBadSignature(t *testing.T) {
	c := New(Config{AESKey: testAESKey(), Token: "tok"})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + DefaultPath + "?msg_signature=wrong&timestamp=1&nonce=n&echostr=e")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestHandlerGetBadEchostr(t *testing.T) {
	token := "tok"
	c := New(Config{AESKey: testAESKey(), Token: token})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	// echostr that passes signature but is not valid base64/ciphertext.
	echo := "not-base64-$$$"
	ts, nonce := "100", "n1"
	sig := signature(token, ts, nonce, echo)
	q := url.Values{"msg_signature": {sig}, "timestamp": {ts}, "nonce": {nonce}, "echostr": {echo}}
	resp, err := http.Get(srv.URL + DefaultPath + "?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	c := New(Config{AESKey: testAESKey(), Token: "tok"})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+DefaultPath, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestHandlerPostBadBody(t *testing.T) {
	c := New(Config{AESKey: testAESKey(), Token: "tok"})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	resp, err := http.Post(srv.URL+DefaultPath, "text/xml", strings.NewReader("not xml"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandlerPostBadSignature(t *testing.T) {
	token := "tok"
	c := New(Config{AESKey: testAESKey(), Token: token})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	inner := `<xml><FromUserName>u1</FromUserName><MsgType>text</MsgType><Content>hi</Content><MsgId>1</MsgId></xml>`
	body, _, ts, nonce := signedInbound(t, c, token, inner)
	// Send a deliberately wrong signature.
	q := url.Values{"msg_signature": {"wrong"}, "timestamp": {ts}, "nonce": {nonce}}
	resp, err := http.Post(srv.URL+DefaultPath+"?"+q.Encode(), "text/xml", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestHandlerPostDecryptFailure(t *testing.T) {
	token := "tok"
	// Channel WITHOUT an AES key => decrypt fails even with a matching signature.
	c := New(Config{Token: token})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	enc := "c29tZS1lbmNyeXB0ZWQtYmxvYg==" // arbitrary base64
	xmlBody, _ := xml.Marshal(struct {
		XMLName xml.Name `xml:"xml"`
		Encrypt string   `xml:"Encrypt"`
	}{Encrypt: enc})
	ts, nonce := "100", "n1"
	sig := signature(token, ts, nonce, enc)
	q := url.Values{"msg_signature": {sig}, "timestamp": {ts}, "nonce": {nonce}}
	resp, err := http.Post(srv.URL+DefaultPath+"?"+q.Encode(), "text/xml", strings.NewReader(string(xmlBody)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandlerPostReceiveIDMismatch(t *testing.T) {
	token := "tok"
	// CorpID set on the channel, but the encrypted payload carries "corpid"
	// as its trailing receive_id => mismatch => forbidden.
	c := New(Config{AESKey: testAESKey(), Token: token, CorpID: "OTHER_CORP"})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	inner := `<xml><FromUserName>u1</FromUserName><MsgType>text</MsgType><Content>hi</Content><MsgId>1</MsgId></xml>`
	body, sig, ts, nonce := signedInbound(t, c, token, inner)
	q := url.Values{"msg_signature": {sig}, "timestamp": {ts}, "nonce": {nonce}}
	resp, err := http.Post(srv.URL+DefaultPath+"?"+q.Encode(), "text/xml", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for receive_id mismatch", resp.StatusCode)
	}
}

func TestHandlerPostDispatches(t *testing.T) {
	token := "tok"
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/gettoken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "T", "expires_in": 7200})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0})
	}))
	defer apiSrv.Close()

	done := make(chan struct{}, 1)
	c := New(Config{
		AESKey:     testAESKey(),
		Token:      token,
		Allowlist:  channel.NewAllowlist([]string{"u1"}),
		APIBase:    apiSrv.URL,
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			done <- struct{}{}
			return channel.Reply{Text: "pong"}, nil
		},
	})
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	inner := `<xml><FromUserName>u1</FromUserName><MsgType>text</MsgType><Content>hi</Content><MsgId>1</MsgId></xml>`
	body, sig, ts, nonce := signedInbound(t, c, token, inner)
	q := url.Values{"msg_signature": {sig}, "timestamp": {ts}, "nonce": {nonce}}
	resp, err := http.Post(srv.URL+DefaultPath+"?"+q.Encode(), "text/xml", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not invoked")
	}
}

func TestDispatchGuards(t *testing.T) {
	c := New(Config{})
	c.dispatch(context.Background(), inbound{sender: ""})
	c.dispatch(context.Background(), inbound{sender: "u1", text: "  "})
	c.seenBefore("dup")
	c.dispatch(context.Background(), inbound{sender: "u1", text: "hi", id: "dup"})
}

func TestDispatchNotAllowedNoReply(t *testing.T) {
	var called bool
	c := New(Config{
		Allowlist: channel.NewAllowlist([]string{"other"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			called = true
			return channel.Reply{Text: "x"}, nil
		},
	})
	c.dispatch(context.Background(), inbound{sender: "u1", text: "hi", id: "e1"})
	if called {
		t.Fatal("handler must not run for non-allowlisted sender")
	}
}

func TestDispatchHandlerErrorSendsApology(t *testing.T) {
	var apiSrv *httptest.Server
	var sentBody string
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/gettoken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "T", "expires_in": 7200})
			return
		}
		b, _ := io.ReadAll(r.Body)
		sentBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0})
	}))
	defer apiSrv.Close()
	c := New(Config{
		Allowlist:  channel.NewAllowlist([]string{"u1"}),
		APIBase:    apiSrv.URL,
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{}, io.EOF
		},
	})
	c.dispatch(context.Background(), inbound{sender: "u1", text: "hi", id: "e2"})
	if !strings.Contains(sentBody, "sorry") {
		t.Fatalf("expected apology to be sent, got %q", sentBody)
	}
}

func TestDispatchEmptyReplyNoSend(t *testing.T) {
	c := New(Config{
		Allowlist: channel.NewAllowlist([]string{"u1"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{Text: ""}, nil
		},
	})
	c.dispatch(context.Background(), inbound{sender: "u1", text: "hi", id: "e3"})
}

func TestDispatchFetchesImage(t *testing.T) {
	var apiSrv *httptest.Server
	var gotImage bool
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/cgi-bin/gettoken":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "T", "expires_in": 7200})
		case strings.Contains(r.URL.Path, "/media/get"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("PNGBYTES"))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0})
		}
	}))
	defer apiSrv.Close()
	c := New(Config{
		Allowlist:  channel.NewAllowlist([]string{"u1"}),
		APIBase:    apiSrv.URL,
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			if len(m.Images) == 1 && strings.HasPrefix(m.Images[0], "data:image/png;base64,") {
				gotImage = true
			}
			return channel.Reply{}, nil
		},
	})
	c.dispatch(context.Background(), inbound{sender: "u1", id: "e4", mediaID: "MID", mediaType: "image"})
	if !gotImage {
		t.Fatal("image media not attached")
	}
}

func TestDispatchFetchesAudio(t *testing.T) {
	var apiSrv *httptest.Server
	var gotAudio bool
	apiSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/cgi-bin/gettoken":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "T", "expires_in": 7200})
		case strings.Contains(r.URL.Path, "/media/get"):
			// Binary body, no explicit content type (defaults to audio/amr).
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte{0x00, 0x01, 0x02, 0x03})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0})
		}
	}))
	defer apiSrv.Close()
	c := New(Config{
		Allowlist:  channel.NewAllowlist([]string{"u1"}),
		APIBase:    apiSrv.URL,
		HTTPClient: apiSrv.Client(),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			if len(m.Audio) == 1 && strings.HasPrefix(m.Audio[0], "data:") && len(m.Images) == 0 {
				gotAudio = true
			}
			return channel.Reply{}, nil
		},
	})
	c.dispatch(context.Background(), inbound{sender: "u1", id: "e5", mediaID: "MID", mediaType: "audio"})
	if !gotAudio {
		t.Fatal("audio media not attached")
	}
}

func TestFetchMediaGuards(t *testing.T) {
	c := New(Config{})
	if got := c.fetchMedia(context.Background(), "", "image"); got != "" {
		t.Fatalf("empty mediaID => %q", got)
	}
}

func TestFetchMediaTokenFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 40001})
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if got := c.fetchMedia(context.Background(), "MID", "image"); got != "" {
		t.Fatalf("token failure => %q", got)
	}
}

func TestFetchMediaNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/gettoken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "T", "expires_in": 7200})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if got := c.fetchMedia(context.Background(), "MID", "image"); got != "" {
		t.Fatalf("non-2xx => %q", got)
	}
}

func TestFetchMediaRejectsJSONError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/gettoken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "T", "expires_in": 7200})
			return
		}
		// WeCom returns a JSON error body instead of binary on failure.
		_, _ = w.Write([]byte(`{"errcode":40007,"errmsg":"invalid media_id"}`))
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if got := c.fetchMedia(context.Background(), "MID", "image"); got != "" {
		t.Fatalf("JSON error body should be rejected, got %q", got)
	}
}

func TestFetchMediaEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/gettoken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "T", "expires_in": 7200})
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if got := c.fetchMedia(context.Background(), "MID", "image"); got != "" {
		t.Fatalf("empty body => %q", got)
	}
}

func TestSendRequiresUser(t *testing.T) {
	c := New(Config{})
	if err := c.Send(context.Background(), channel.Outbound{Text: "hi"}); err == nil {
		t.Fatal("expected error without a user id")
	}
}

func TestSendEmptyTextNoop(t *testing.T) {
	c := New(Config{})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "u1", Text: "  "}); err != nil {
		t.Fatalf("empty text should be a no-op, got %v", err)
	}
}

func TestSendTokenError(t *testing.T) {
	c := New(Config{APIBase: "http://127.0.0.1:0", HTTPClient: &http.Client{Timeout: 200 * time.Millisecond}})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "u1", Text: "hi"}); err == nil {
		t.Fatal("expected token error")
	}
}

func TestSendChunksAndPublishes(t *testing.T) {
	var sendHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/gettoken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "T", "expires_in": 7200})
			return
		}
		sendHits++
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0})
	}))
	defer srv.Close()
	long := strings.Repeat("a", wecomMaxChars+50)
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client(), Bus: newBus(t)})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "u1", Text: long}); err != nil {
		t.Fatal(err)
	}
	if sendHits < 2 {
		t.Fatalf("expected >=2 chunk sends, got %d", sendHits)
	}
}

func TestSendOneNon2xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/gettoken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "T", "expires_in": 7200})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "u1", Text: "hi"}); err == nil {
		t.Fatal("expected non-2xx send error")
	}
}

func TestAccessTokenErrorPaths(t *testing.T) {
	// Unmarshal failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<<not json>>"))
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if _, err := c.accessToken(context.Background()); err == nil {
		t.Fatal("expected unmarshal error")
	}

	// Empty token with errcode.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 40013})
	}))
	defer srv2.Close()
	c2 := New(Config{APIBase: srv2.URL, HTTPClient: srv2.Client()})
	if _, err := c2.accessToken(context.Background()); err == nil {
		t.Fatal("expected token-failed error")
	}
}

func TestAccessTokenCachedWithDefaultExpiry(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "T", "expires_in": 0})
	}))
	defer srv.Close()
	c := New(Config{APIBase: srv.URL, HTTPClient: srv.Client()})
	if tok, err := c.accessToken(context.Background()); err != nil || tok != "T" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	if _, err := c.accessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("token fetched %d times, want 1", hits)
	}
}

func TestEmitInboundNilBus(t *testing.T) {
	New(Config{}).emitInbound(channel.UnifiedMessage{}, "corr", true)
}

func TestEmitInboundWithBus(t *testing.T) {
	c := New(Config{Bus: newBus(t)})
	c.emitInbound(channel.UnifiedMessage{ChannelID: "u1", Sender: "u1", Text: "hi"}, "corr", true)
}

func TestSeenBeforeRingEviction(t *testing.T) {
	c := New(Config{})
	for i := 0; i < dedupCapacity+5; i++ {
		if c.seenBefore("id" + itoa(i)) {
			t.Fatalf("fresh id id%d seen", i)
		}
	}
	if !c.seenBefore("id" + itoa(dedupCapacity+4)) {
		t.Fatal("recent id should be seen")
	}
	if c.seenBefore("id0") {
		t.Fatal("first id should have been evicted")
	}
}

func TestDecryptGuards(t *testing.T) {
	// No AES key configured.
	if _, _, err := New(Config{}).decrypt("anything"); err == nil {
		t.Fatal("expected AES-not-configured error")
	}
	// Bad base64.
	c := New(Config{AESKey: testAESKey()})
	if _, _, err := c.decrypt("!!!not-base64!!!"); err == nil {
		t.Fatal("expected base64 error")
	}
	// Ciphertext not a block multiple.
	if _, _, err := c.decrypt("YWJj"); err == nil { // "abc" => 3 bytes
		t.Fatal("expected bad-ciphertext-length error")
	}
}

func TestPkcs7UnpadGuards(t *testing.T) {
	if _, err := pkcs7Unpad(nil); err == nil {
		t.Fatal("empty input should error")
	}
	// Pad byte out of range.
	if _, err := pkcs7Unpad([]byte{0x01, 0xFF}); err == nil {
		t.Fatal("bad pad should error")
	}
	// Valid pad.
	if out, err := pkcs7Unpad([]byte{0x41, 0x02, 0x02}); err != nil || string(out) != "A" {
		t.Fatalf("unpad = %q err=%v", out, err)
	}
}

func TestParseMessageInvalidXML(t *testing.T) {
	if _, ok := parseMessage([]byte("<not-closed")); ok {
		t.Fatal("invalid XML should return ok=false")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
