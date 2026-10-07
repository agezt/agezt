// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryOAuthClient struct {
	body    string
	status  int
	err     error
	request *http.Request
	form    url.Values
	closed  bool
}
type memoryOAuthBody struct {
	io.Reader
	owner *memoryOAuthClient
}

type erroredOAuthBody struct {
	data   []byte
	closed bool
}

func (b *erroredOAuthBody) Read(p []byte) (int, error) {
	if len(b.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, b.data)
	b.data = b.data[n:]
	return n, errors.New("owned read failure")
}
func (b *erroredOAuthBody) Close() error { b.closed = true; return nil }

type fixedOAuthResponse struct{ body io.ReadCloser }

func (c fixedOAuthResponse) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: c.body}, nil
}

func (b memoryOAuthBody) Close() error { b.owner.closed = true; return nil }
func (p *memoryOAuthClient) Do(r *http.Request) (*http.Response, error) {
	p.request = r
	raw, _ := io.ReadAll(r.Body)
	p.form, _ = url.ParseQuery(string(raw))
	if p.err != nil {
		return nil, p.err
	}
	return &http.Response{StatusCode: p.status, Header: http.Header{}, Body: memoryOAuthBody{Reader: strings.NewReader(p.body), owner: p}}, nil
}

func TestChannelOAuthMemoryProviderTTLValueSnapshotsAndLegacyRetention(t *testing.T) {
	p := NewOAuthMemory(nil, nil)
	now := time.Now()
	slack, ok := p.Provider("slack")
	if !ok || !reflect.DeepEqual(slack, OAuthProvider{AuthURL: "https://slack.com/oauth/v2/authorize", TokenURL: "https://slack.com/api/oauth.v2.access", Scopes: "chat:write,channels:read", TokenEnv: "AGEZT_SLACK_TOKEN"}) {
		t.Fatal(slack, ok)
	}
	if mastodon, ok := p.Provider("mastodon"); !ok || !mastodon.InstanceBased || mastodon.TokenEnv != "AGEZT_MASTODON_TOKEN" || mastodon.Scopes != "read write" {
		t.Fatal(mastodon, ok)
	}
	if _, ok := p.Provider("unknown"); ok {
		t.Fatal("unknown provider")
	}
	old := now.Add(-OAuthFlowTTL - time.Nanosecond)
	edge := now.Add(-OAuthFlowTTL)
	p.Put("old", OAuthFlow{Created: old, Status: "pending"}, old)
	p.Put("edge", OAuthFlow{Created: edge, Status: "pending"}, edge)
	if _, ok := p.Flow("old"); !ok {
		t.Fatal("lookup unexpectedly prunes without start")
	}
	p.Put("fresh", OAuthFlow{Created: now, Status: "pending", ClientSecret: "owned secret"}, now)
	if _, ok := p.Flow("old"); ok {
		t.Fatal("expired state survived start")
	}
	if _, ok := p.Flow("edge"); !ok {
		t.Fatal("TTL equality removed")
	}
	copy, ok := p.Flow("fresh")
	if !ok {
		t.Fatal("fresh missing")
	}
	copy.ClientSecret = "caller changed"
	copy.Status = "caller changed"
	p.SetStatus("fresh", "done", "")
	fresh, _ := p.Flow("fresh")
	if fresh.ClientSecret != "owned secret" || fresh.Status != "done" {
		t.Fatal(fresh)
	}
	p.SetStatus("fresh", "error", " owned message ")
	fresh, _ = p.Flow("fresh")
	if fresh.Status != "error" || fresh.Error != " owned message " {
		t.Fatal(fresh)
	}
	p.SetStatus("missing", "done", "")
	if _, ok := p.Flow("missing"); ok {
		t.Fatal("missing status created state")
	}
	q := NewOAuthMemory(nil, nil)
	if _, ok := q.Flow("fresh"); ok {
		t.Fatal("state shared across owners")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				p.SetStatus("fresh", "done", "done")
				f, _ := p.Flow("fresh")
				if f.Status == "done" && f.Error != "done" {
					t.Error("non-atomic snapshot")
				}
			}
		}()
	}
	wg.Wait()
	// Credential-bearing internal Flow has no public operation terminal binding.
	raw, _ := json.Marshal(OAuthStatusOutput{"status": fresh.Status, "error": fresh.Error, "kind": fresh.Kind, "label": fresh.Label})
	if strings.Contains(string(raw), "owned secret") {
		t.Fatal("status exposed internal credentials")
	}
}

func TestChannelOAuthMemoryExchangeMovedHeadersFormErrorPrecedenceAndContext(t *testing.T) {
	flow := OAuthFlow{ClientID: " raw id ", ClientSecret: " raw secret ", RedirectURI: "https://owned.example/cb", TokenURL: "https://owned.example/token"}
	for _, tc := range []struct {
		body                 string
		status               int
		wantToken, wantError string
	}{
		{`{"ok":true,"access_token":" raw token "}`, 200, " raw token ", ""},
		{`{"access_token":"owned"}`, 503, "owned", ""},
		{`{"ok":false,"access_token":"ignored","error":"code","error_description":"desc"}`, 400, "", "desc"},
		{`{"error":"code"}`, 400, "", "code"},
		{`{}`, 201, "", "no access_token in response (status 201)"},
		{"non-json", 500, "", "provider returned non-JSON (status 500)"},
	} {
		client := &memoryOAuthClient{body: tc.body, status: tc.status}
		var timeout time.Duration
		p := NewOAuthMemory(func(v time.Duration) OAuthHTTPClient { timeout = v; return client }, nil)
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "owned")
		start := time.Now()
		token, err := p.Exchange(ctx, flow, " raw code ")
		if token != tc.wantToken || tc.wantError == "" && err != nil || tc.wantError != "" && (err == nil || err.Error() != tc.wantError) {
			t.Fatal(token, err)
		}
		if timeout != 20*time.Second || !client.closed || client.request.Method != http.MethodPost || client.request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || client.request.Header.Get("Accept") != "application/json" || client.request.Context().Value(key{}) != "owned" {
			t.Fatal("request/client/body contract")
		}
		deadline, ok := client.request.Context().Deadline()
		if !ok || deadline.Before(start.Add(19*time.Second)) || deadline.After(time.Now().Add(20*time.Second)) {
			t.Fatal("exchange timeout")
		}
		if !reflect.DeepEqual(client.form, url.Values{"grant_type": {"authorization_code"}, "code": {" raw code "}, "redirect_uri": {flow.RedirectURI}, "client_id": {flow.ClientID}, "client_secret": {flow.ClientSecret}}) {
			t.Fatal("form changed")
		}
		if !errors.Is(client.request.Context().Err(), context.Canceled) {
			t.Fatal("exchange context not released")
		}
	}
	sentinel := errors.New("owned transport failure")
	client := &memoryOAuthClient{err: sentinel}
	p := NewOAuthMemory(func(time.Duration) OAuthHTTPClient { return client }, nil)
	if _, err := p.Exchange(context.Background(), flow, "code"); !errors.Is(err, sentinel) {
		t.Fatal("transport cause lost", err)
	}
	flow.TokenURL = ":bad URL"
	if _, err := p.Exchange(context.Background(), flow, "code"); err == nil {
		t.Fatal("malformed request accepted")
	}
}

func TestChannelOAuthMemoryExchangeBodyLimitAndLegacyIgnoredReadError(t *testing.T) {
	flow := OAuthFlow{TokenURL: "https://owned.example/token"}
	readBody := &erroredOAuthBody{data: []byte(`{"access_token":"owned-token"}`)}
	p := NewOAuthMemory(func(time.Duration) OAuthHTTPClient { return fixedOAuthResponse{body: readBody} }, nil)
	if token, err := p.Exchange(context.Background(), flow, "owned-code"); err != nil || token != "owned-token" || !readBody.closed {
		t.Fatal("legacy read-error behavior changed", token, err)
	}
	oversized := &memoryOAuthClient{status: 200, body: `{"access_token":"` + strings.Repeat("x", 1<<20) + `"}`}
	p = NewOAuthMemory(func(time.Duration) OAuthHTTPClient { return oversized }, nil)
	if _, err := p.Exchange(context.Background(), flow, "owned-code"); err == nil || err.Error() != "provider returned non-JSON (status 200)" || !oversized.closed {
		t.Fatal("body bound changed", err)
	}
}
