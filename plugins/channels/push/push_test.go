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
