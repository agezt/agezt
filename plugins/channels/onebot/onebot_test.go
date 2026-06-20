// SPDX-License-Identifier: MIT

package onebot

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestParseEvent(t *testing.T) {
	priv := []byte(`{"post_type":"message","message_type":"private","message_id":42,"user_id":12345,"raw_message":"hi"}`)
	m, ok := parseEvent(priv)
	if !ok || m.sender != "12345" || m.target != "private:12345" || m.text != "hi" || m.id != "42" {
		t.Fatalf("private = %+v ok=%v", m, ok)
	}
	grp := []byte(`{"post_type":"message","message_type":"group","message_id":7,"user_id":1,"group_id":999,"message":"yo"}`)
	g, ok := parseEvent(grp)
	if !ok || g.target != "group:999" || g.sender != "1" {
		t.Fatalf("group = %+v ok=%v", g, ok)
	}
	// non-message post types dropped.
	if _, ok := parseEvent([]byte(`{"post_type":"notice"}`)); ok {
		t.Fatal("notice should be dropped")
	}
	// CQ image/record codes are pulled out as media (URL decoded) and stripped from text.
	cq := []byte(`{"post_type":"message","message_type":"private","message_id":9,"user_id":5,"raw_message":"look [CQ:image,file=a.jpg,url=https://x.test/a.jpg] and [CQ:record,url=https://x.test/v.amr]"}`)
	c, ok := parseEvent(cq)
	if !ok || len(c.media) != 2 {
		t.Fatalf("cq media = %+v ok=%v", c, ok)
	}
	if c.media[0].kind != "image" || c.media[0].url != "https://x.test/a.jpg" {
		t.Fatalf("image media = %+v", c.media[0])
	}
	if c.media[1].kind != "audio" || c.media[1].url != "https://x.test/v.amr" {
		t.Fatalf("record media = %+v", c.media[1])
	}
	if strings.Contains(c.text, "CQ:") {
		t.Fatalf("CQ codes should be stripped from text: %q", c.text)
	}
}

func TestExtractCQMediaUnescape(t *testing.T) {
	_, media := extractCQMedia(`[CQ:image,url=https://x.test/a&#44;b.jpg&amp;k=1]`)
	if len(media) != 1 || media[0].url != "https://x.test/a,b.jpg&k=1" {
		t.Fatalf("unescape = %+v", media)
	}
}

func TestSplitTarget(t *testing.T) {
	cases := map[string][2]string{
		"private:5": {"private", "5"},
		"group:9":   {"group", "9"},
		"123":       {"private", "123"},
	}
	for in, want := range cases {
		mt, id := splitTarget(in)
		if mt != want[0] || id != want[1] {
			t.Fatalf("splitTarget(%q) = %q,%q", in, mt, id)
		}
	}
}

func TestFetchMediaSSRFGuard(t *testing.T) {
	// The CQ media URL is attacker-controlled, so fetchMedia must refuse to reach
	// loopback/private targets. An httptest server binds 127.0.0.1, which the
	// guarded client blocks → "".
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG"))
	}))
	defer srv.Close()
	c := New(Config{Kind: "qq"})
	if du := c.fetchMedia(context.Background(), srv.URL); du != "" {
		t.Fatalf("loopback media URL should be blocked, got %q", du)
	}
	if hit {
		t.Fatal("guarded client connected to a loopback server")
	}
	// Non-http(s) schemes are rejected outright.
	if du := c.fetchMedia(context.Background(), "file:///etc/passwd"); du != "" {
		t.Fatalf("file scheme should be rejected, got %q", du)
	}
}

func TestValidSignature(t *testing.T) {
	secret := "s"
	body := []byte(`{"post_type":"message"}`)
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	sig := "sha1=" + hex.EncodeToString(mac.Sum(nil))
	if !validSignature(secret, body, sig) {
		t.Fatal("valid signature rejected")
	}
	if validSignature(secret, body, "sha1=deadbeef") {
		t.Fatal("bad signature accepted")
	}
}

func TestDispatchRepliesViaGateway(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	ch := New(Config{
		Kind:      "qq",
		APIBase:   srv.URL,
		Allowlist: channel.NewAllowlist([]string{"12345"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{Text: "pong"}, nil
		},
		HTTPClient: srv.Client(),
	})
	ch.dispatch(context.Background(), inbound{sender: "12345", target: "private:12345", text: "ping", id: "1"})
	var p map[string]any
	_ = json.Unmarshal([]byte(body), &p)
	if p["message_type"] != "private" || p["message"] != "pong" {
		t.Fatalf("reply body = %s", body)
	}
	if _, ok := p["user_id"].(float64); !ok {
		t.Fatalf("numeric user_id expected: %s", body)
	}
}
