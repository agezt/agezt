// SPDX-License-Identifier: MIT

package push

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestNewValidation(t *testing.T) {
	if _, err := New(Config{Kind: "nope"}); err == nil {
		t.Fatal("unknown kind must error")
	}
	if _, err := New(Config{Kind: KindPushover, Token: "t"}); err == nil {
		t.Fatal("pushover without user must error")
	}
	if _, err := New(Config{Kind: KindNtfy, Topic: "alerts"}); err != nil {
		t.Fatalf("valid ntfy should construct: %v", err)
	}
	if _, err := New(Config{Kind: KindGoogleChat, URL: "https://x"}); err != nil {
		t.Fatalf("valid googlechat should construct: %v", err)
	}
}

func TestSendNtfy(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer srv.Close()
	ch, err := New(Config{Kind: KindNtfy, Server: srv.URL, Topic: "alerts", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Send(context.Background(), channel.Outbound{Text: "hello"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotPath != "/alerts" || gotBody != "hello" {
		t.Fatalf("ntfy posted path=%q body=%q", gotPath, gotBody)
	}
}

func TestSendGoogleChatJSON(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer srv.Close()
	ch, _ := New(Config{Kind: KindGoogleChat, URL: srv.URL, HTTPClient: srv.Client()})
	if err := ch.Send(context.Background(), channel.Outbound{Text: "hi team"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(gotBody, `"text":"hi team"`) {
		t.Fatalf("googlechat body = %q", gotBody)
	}
}

func TestLineRequest(t *testing.T) {
	// Line posts to a fixed api.line.me URL; inspect the built request directly.
	ch, err := New(Config{Kind: KindLine, Token: "tok", Target: "U123"})
	if err != nil {
		t.Fatal(err)
	}
	req, err := ch.buildRequest(context.Background(), "hey")
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.String() != "https://api.line.me/v2/bot/message/push" {
		t.Fatalf("line url = %s", req.URL)
	}
	if req.Header.Get("Authorization") != "Bearer tok" {
		t.Fatalf("line auth = %q", req.Header.Get("Authorization"))
	}
	b, _ := io.ReadAll(req.Body)
	if !strings.Contains(string(b), `"to":"U123"`) || !strings.Contains(string(b), `"text":"hey"`) {
		t.Fatalf("line body = %q", string(b))
	}
}

func TestSendZulipBasicAuth(t *testing.T) {
	var user, pass, gotForm string
	var ok bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok = r.BasicAuth()
		b, _ := io.ReadAll(r.Body)
		gotForm = string(b)
	}))
	defer srv.Close()
	ch, err := New(Config{Kind: KindZulip, Server: srv.URL, User: "bot@x", Token: "key", Target: "general", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Send(context.Background(), channel.Outbound{Text: "ping"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !ok || user != "bot@x" || pass != "key" {
		t.Fatalf("zulip basic auth = %q/%q ok=%v", user, pass, ok)
	}
	if !strings.Contains(gotForm, "content=ping") || !strings.Contains(gotForm, "topic=agezt") {
		t.Fatalf("zulip form = %q", gotForm)
	}
}

func TestSendFeishuAndDingTalkShapes(t *testing.T) {
	cases := []struct {
		kind, want string
	}{
		{KindFeishu, `"msg_type":"text"`},
		{KindDingTalk, `"msgtype":"text"`},
		{KindWeCom, `"msgtype":"text"`},
	}
	for _, c := range cases {
		var body string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
		}))
		ch, err := New(Config{Kind: c.kind, URL: srv.URL, HTTPClient: srv.Client()})
		if err != nil {
			t.Fatalf("%s new: %v", c.kind, err)
		}
		if err := ch.Send(context.Background(), channel.Outbound{Text: "hi"}); err != nil {
			t.Fatalf("%s send: %v", c.kind, err)
		}
		if !strings.Contains(body, c.want) || !strings.Contains(body, "hi") {
			t.Fatalf("%s body = %q want %q", c.kind, body, c.want)
		}
		srv.Close()
	}
}

func TestSendSynologyForm(t *testing.T) {
	var ctype, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctype = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	ch, _ := New(Config{Kind: KindSynology, URL: srv.URL, HTTPClient: srv.Client()})
	if err := ch.Send(context.Background(), channel.Outbound{Text: "hello"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(ctype, "x-www-form-urlencoded") || !strings.Contains(body, "payload=") {
		t.Fatalf("synology ctype=%q body=%q", ctype, body)
	}
}

func TestSendEmptyIsNoop(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()
	ch, _ := New(Config{Kind: KindNtfy, Server: srv.URL, Topic: "t", HTTPClient: srv.Client()})
	if err := ch.Send(context.Background(), channel.Outbound{Text: "  "}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("empty text should not POST")
	}
}
