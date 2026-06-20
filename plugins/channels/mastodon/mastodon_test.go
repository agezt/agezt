// SPDX-License-Identifier: MIT

package mastodon

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestStripHTML(t *testing.T) {
	in := `<p>hello <span class="h-card"><a href="x">@bot</a></span> &amp; friends<br>line two</p>`
	got := stripHTML(in)
	want := "hello @bot & friends\nline two"
	if got != want {
		t.Fatalf("stripHTML = %q, want %q", got, want)
	}
}

func TestSendPostsStatus(t *testing.T) {
	var gotStatus, gotReplyTo, gotAuth, gotVis string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(b))
		gotStatus = vals.Get("status")
		gotReplyTo = vals.Get("in_reply_to_id")
		gotVis = vals.Get("visibility")
		_, _ = io.WriteString(w, `{"id":"99"}`)
	}))
	defer srv.Close()

	c := New(Config{Server: srv.URL, Token: "tok", HTTPClient: srv.Client()})
	// Standalone post (brief): no reply target.
	if err := c.Send(context.Background(), channel.Outbound{Text: "hello world"}); err != nil {
		t.Fatal(err)
	}
	if gotStatus != "hello world" || gotReplyTo != "" || gotAuth != "Bearer tok" || gotVis != "unlisted" {
		t.Fatalf("status=%q reply=%q auth=%q vis=%q", gotStatus, gotReplyTo, gotAuth, gotVis)
	}
}

func TestDispatchThreadsReplyAndAllowlists(t *testing.T) {
	var gotStatus, gotReplyTo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(b))
		gotStatus = vals.Get("status")
		gotReplyTo = vals.Get("in_reply_to_id")
		_, _ = io.WriteString(w, `{"id":"100"}`)
	}))
	defer srv.Close()

	c := New(Config{
		Server:     srv.URL,
		Token:      "tok",
		Allowlist:  channel.NewAllowlist([]string{"alice@example.social"}),
		HTTPClient: srv.Client(),
		Handler: func(_ context.Context, msg channel.UnifiedMessage, _ string) (channel.Reply, error) {
			return channel.Reply{Text: "got: " + msg.Text}, nil
		},
	})

	var n notification
	n.Status.ID = "555"
	n.Status.Content = "<p>@bot ping</p>"
	n.Status.Visibility = "public"
	n.Status.Account.Acct = "alice@example.social"
	c.dispatch(context.Background(), n)
	if gotReplyTo != "555" {
		t.Fatalf("reply should thread to status 555, got %q", gotReplyTo)
	}
	if !strings.HasPrefix(gotStatus, "@alice@example.social ") || !strings.Contains(gotStatus, "got: @bot ping") {
		t.Fatalf("reply status = %q", gotStatus)
	}

	// A sender not on the allowlist must not get a reply.
	gotStatus, gotReplyTo = "", ""
	var n2 notification
	n2.Status.ID = "777"
	n2.Status.Content = "<p>@bot hi</p>"
	n2.Status.Account.Acct = "mallory@evil.social"
	c.dispatch(context.Background(), n2)
	if gotStatus != "" {
		t.Fatalf("non-allowlisted should not reply, got %q", gotStatus)
	}
}

func TestPollAdvancesCursorOldestFirst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Newest-first, as the real API returns.
		_, _ = io.WriteString(w, `[
		  {"id":"20","type":"mention","status":{"id":"s20","content":"<p>two</p>","account":{"acct":"a"}}},
		  {"id":"10","type":"mention","status":{"id":"s10","content":"<p>one</p>","account":{"acct":"a"}}}
		]`)
	}))
	defer srv.Close()
	c := New(Config{Server: srv.URL, Token: "tok", HTTPClient: srv.Client(), Allowlist: channel.NewAllowlist(nil)})
	c.poll(context.Background())
	if c.since != "20" {
		t.Fatalf("cursor = %q, want newest id 20", c.since)
	}
}
