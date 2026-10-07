// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

type oauthProbe struct {
	provider                                 OAuthProvider
	providerFound, flowFound                 bool
	flow                                     OAuthFlow
	state, seenState, kind, code, key, token string
	now                                      time.Time
	calls                                    []string
	stateErr, exchangeErr, loadErr, saveErr  error
	ctx                                      context.Context
	deadline                                 time.Time
	status, message                          string
}

func (p *oauthProbe) Provider(kind string) (OAuthProvider, bool) {
	p.calls = append(p.calls, "provider")
	p.kind = kind
	return p.provider, p.providerFound
}
func (p *oauthProbe) NewState() (string, error) {
	p.calls = append(p.calls, "state")
	return p.state, p.stateErr
}
func (p *oauthProbe) Now() time.Time { p.calls = append(p.calls, "now"); return p.now }
func (p *oauthProbe) Put(state string, f OAuthFlow, now time.Time) {
	p.calls = append(p.calls, "put")
	p.seenState = state
	p.flow = f
	if f.Created != now {
		panic("created/put mismatch")
	}
}
func (p *oauthProbe) Flow(state string) (OAuthFlow, bool) {
	p.calls = append(p.calls, "flow")
	p.seenState = state
	return p.flow, p.flowFound
}
func (p *oauthProbe) SetStatus(state, status, msg string) {
	p.calls = append(p.calls, "status")
	p.seenState, p.status, p.message = state, status, msg
}
func (p *oauthProbe) Exchange(ctx context.Context, f OAuthFlow, code string) (string, error) {
	p.calls = append(p.calls, "exchange")
	p.ctx = ctx
	p.deadline, _ = ctx.Deadline()
	p.code = code
	if !reflect.DeepEqual(f, p.flow) {
		panic("exchange flow changed")
	}
	return p.token, p.exchangeErr
}
func (p *oauthProbe) Vault() AccountStore { p.calls = append(p.calls, "vault"); return p }
func (p *oauthProbe) Load() error         { p.calls = append(p.calls, "load"); return p.loadErr }
func (p *oauthProbe) Set(key, token string) {
	p.calls = append(p.calls, "set")
	p.key, p.token = key, token
}
func (p *oauthProbe) Remove(string) bool { panic("unexpected remove") }
func (p *oauthProbe) Save() error        { p.calls = append(p.calls, "save"); return p.saveErr }
func newOAuthProbe() *oauthProbe {
	return &oauthProbe{providerFound: true, flowFound: true, provider: OAuthProvider{AuthURL: "https://owned.example/authorize", TokenURL: "https://owned.example/token", Scopes: "scope a", TokenEnv: "AGEZT_OWNED_TOKEN"}, flow: OAuthFlow{Kind: "slack", Label: "work", ClientID: "raw id", ClientSecret: "raw secret", RedirectURI: "https://owned.example/cb", TokenURL: "https://owned.example/token", TokenEnv: "AGEZT_OWNED_TOKEN", Status: "pending"}, state: "owned state", token: "owned token", now: time.Unix(1700000000, 0)}
}
func oauthCalls(t *testing.T, p *oauthProbe, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(p.calls, want) {
		t.Fatal(p.calls, want)
	}
}

func TestChannelOAuthStartOrderTrimURLsStateAndLegacyCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, instance := range []bool{false, true} {
		p := newOAuthProbe()
		p.provider.InstanceBased = instance
		out, err := NewOAuth(p).Start(ctx, OAuthStartInput{Kind: " SLACK ", Label: " work ", ClientID: " owned id ", ClientSecret: " owned secret ", RedirectURI: " https://owned.example/cb?raw=yes ", InstanceURL: " owned.example/path?query=ignored "})
		if err != nil || out.State != p.state || p.kind != "slack" || p.seenState != p.state {
			t.Fatal(out, err, p)
		}
		parsed, err := url.Parse(out.AuthorizeURL)
		if err != nil {
			t.Fatal(err)
		}
		wantPath := "/authorize"
		wantToken := "https://owned.example/token"
		if instance {
			wantPath = "/oauth/authorize"
			wantToken = "https://owned.example/oauth/token"
		}
		query := parsed.Query()
		if parsed.Path != wantPath || len(query) != 5 || query.Get("client_id") != "owned id" || query.Get("redirect_uri") != "https://owned.example/cb?raw=yes" || query.Get("response_type") != "code" || query.Get("scope") != "scope a" || query.Get("state") != p.state {
			t.Fatal(parsed)
		}
		if p.flow.Kind != "slack" || p.flow.Label != "work" || p.flow.ClientID != "owned id" || p.flow.ClientSecret != "owned secret" || p.flow.RedirectURI != "https://owned.example/cb?raw=yes" || p.flow.TokenURL != wantToken || p.flow.TokenEnv != "AGEZT_OWNED_TOKEN" || p.flow.Status != "pending" || p.flow.Error != "" || p.flow.Created != p.now {
			t.Fatal(p.flow)
		}
		oauthCalls(t, p, "provider", "state", "now", "put")
	}
	for _, tc := range []struct {
		in          OAuthStartInput
		want        string
		unsupported bool
	}{
		{OAuthStartInput{Kind: "unknown", Label: "Bad Label"}, "unknown does not support OAuth connect", true},
		{OAuthStartInput{Kind: "slack", Label: "Bad Label"}, "label must be a slug: lowercase letters/digits/-/_, max 32", false},
		{OAuthStartInput{Kind: "slack"}, "client_id and client_secret are required", false},
		{OAuthStartInput{Kind: "slack", ClientID: "id", ClientSecret: "secret", RedirectURI: "http://owned.example/cb"}, "redirect_uri must be an https:// URL (http is allowed only for loopback dev: localhost, 127.0.0.1, ::1)", false},
	} {
		p := newOAuthProbe()
		p.providerFound = !tc.unsupported
		out, err := NewOAuth(p).Start(ctx, tc.in)
		if out != (OAuthStartOutput{}) || err == nil || err.Error() != tc.want {
			t.Fatal(out, err)
		}
		oauthCalls(t, p, "provider")
	}
	p := newOAuthProbe()
	p.stateErr = errors.New("owned entropy")
	out, err := NewOAuth(p).Start(ctx, OAuthStartInput{Kind: "slack", ClientID: "id", ClientSecret: "secret", RedirectURI: "https://owned.example/cb"})
	if out != (OAuthStartOutput{}) || err == nil || err.Error() != "generate state: owned entropy" || errors.Is(err, p.stateErr) {
		t.Fatal(out, err)
	}
	oauthCalls(t, p, "provider", "state")
}

func TestChannelOAuthCallbackContextVaultOrderErrorsAndRawToken(t *testing.T) {
	for _, phase := range []string{"success", "exchange", "load", "save"} {
		p := newOAuthProbe()
		cause := errors.New("owned cause")
		switch phase {
		case "exchange":
			p.exchangeErr = cause
		case "load":
			p.loadErr = cause
		case "save":
			p.saveErr = cause
		}
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "owned")
		started := time.Now()
		out, err := NewOAuth(p).Callback(ctx, OAuthCallbackInput{Code: " owned code ", State: " owned state "})
		if p.ctx.Value(key{}) != "owned" || p.code != "owned code" || p.seenState != "owned state" || p.deadline.Before(started.Add(19*time.Second)) || p.deadline.After(time.Now().Add(20*time.Second)) {
			t.Fatal("context/code/state/timeout changed", p)
		}
		if phase == "success" {
			if err != nil || !reflect.DeepEqual(out, OAuthCallbackOutput{OK: true, Kind: "slack", Label: "work", Env: "AGEZT_OWNED_TOKEN#work", Applied: "restart"}) || p.status != "done" || p.message != "" || p.key != "AGEZT_OWNED_TOKEN#work" || p.token != "owned token" {
				t.Fatal(out, err, p)
			}
			oauthCalls(t, p, "flow", "exchange", "vault", "load", "set", "save", "status")
		} else {
			prefix := map[string]string{"exchange": "token exchange failed", "load": "load vault", "save": "save vault"}[phase]
			if out != (OAuthCallbackOutput{}) || err == nil || err.Error() != prefix+": owned cause" || errors.Is(err, cause) || p.status != "error" || p.message != "owned cause" {
				t.Fatal(out, err, p)
			}
			want := []string{"flow", "exchange"}
			if phase != "exchange" {
				want = append(want, "vault", "load")
			}
			if phase == "save" {
				want = append(want, "set", "save")
			}
			want = append(want, "status")
			oauthCalls(t, p, want...)
		}
		if !errors.Is(p.ctx.Err(), context.Canceled) {
			t.Fatal("callback timeout context was not released")
		}
	}
	p := newOAuthProbe()
	p.flowFound = false
	if _, err := NewOAuth(p).Callback(context.Background(), OAuthCallbackInput{Code: "code", State: "state"}); err == nil || err.Error() != "unknown or expired state" {
		t.Fatal(err)
	}
	oauthCalls(t, p, "flow")
	p = newOAuthProbe()
	if _, err := NewOAuth(p).Callback(context.Background(), OAuthCallbackInput{}); err == nil || err.Error() != "code and state required" {
		t.Fatal(err)
	}
	if len(p.calls) != 0 {
		t.Fatal(p.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p = newOAuthProbe()
	p.exchangeErr = context.Canceled
	if _, err := NewOAuth(p).Callback(ctx, OAuthCallbackInput{Code: "code", State: "state"}); err == nil || p.ctx.Err() != context.Canceled {
		t.Fatal("caller cancellation lost", err)
	}
	oauthCalls(t, p, "flow", "exchange", "status")
}

func TestChannelOAuthStatusRawUnknownHelpersAndStateRandomness(t *testing.T) {
	p := newOAuthProbe()
	p.flow.Status = " raw status "
	p.flow.Error = " raw error "
	p.flow.Kind = " raw kind "
	p.flow.Label = " raw label "
	out, err := NewOAuth(p).Status(context.Background(), OAuthStatusInput{State: " raw state "})
	if err != nil || out.Status != " raw status " || *out.Error != " raw error " || *out.Kind != " raw kind " || *out.Label != " raw label " || p.seenState != "raw state" {
		t.Fatal(out, err)
	}
	p.flowFound = false
	out, err = NewOAuth(p).Status(context.Background(), OAuthStatusInput{})
	if err != nil || !reflect.DeepEqual(out, OAuthStatusOutput{Status: "unknown"}) {
		t.Fatal(out, err)
	}
	for _, tc := range []struct{ raw, want string }{{"owned.example/path?x=1", "https://owned.example"}, {"http://127.0.0.1:1234/path", "http://127.0.0.1:1234"}, {"https://owned.example:444/path#fragment", "https://owned.example:444"}} {
		got, err := NormalizeInstanceURL(tc.raw)
		if err != nil || got != tc.want {
			t.Fatal(got, err)
		}
	}
	for _, raw := range []string{"", "://bad", "ftp://owned.example"} {
		if _, err := NormalizeInstanceURL(raw); err == nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"https://owned.example/cb", "http://localhost/cb", "http://127.0.0.1/cb", "http://[::1]/cb"} {
		if !IsHTTPSURL(raw) {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"http://owned.example/cb", "http://192.0.2.1/cb", "", "https://", "file:///owned"} {
		if IsHTTPSURL(raw) {
			t.Fatal(raw)
		}
	}
	seen := map[string]bool{}
	for i := 0; i < 32; i++ {
		state, err := NewOAuthState()
		raw, decodeErr := base64.RawURLEncoding.DecodeString(state)
		if err != nil || decodeErr != nil || len(raw) != 32 || strings.ContainsAny(state, "+/=") || seen[state] {
			t.Fatal("state generation contract")
		}
		seen[state] = true
	}
}
