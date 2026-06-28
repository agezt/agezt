// SPDX-License-Identifier: MIT

package controlplane

// Channel OAuth connect flow (Phase 4). For channels whose ConnectMethod is
// "oauth", an operator registers an OAuth app with the provider, pastes the
// client id + secret into the Connect page, and clicks "Connect with X" instead
// of hunting for a token. The daemon builds the provider's authorize URL, the
// browser authorizes and is redirected to the daemon's public /oauth/callback,
// and the daemon exchanges the code for an access token which it writes into the
// account's "#label" vault slot — the same storage every other channel field
// uses. Token paste stays available as a fallback.
//
// Scope: providers whose returned access_token is DIRECTLY usable as the
// channel's token — Slack (v2 returns the bot token at top-level access_token)
// and Mastodon (per-instance user token). Discord (static bot token from the dev
// portal) and Google Chat (service account) don't benefit from user-OAuth and
// stay on token entry.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/netguard"
	"github.com/agezt/agezt/kernel/settings"
)

// oauthProvider describes a channel's OAuth2 authorization-code endpoints. When
// instanceBased, the authorize/token URLs are derived from an operator-supplied
// instance URL (e.g. a Mastodon server) rather than fixed.
type oauthProvider struct {
	authURL       string
	tokenURL      string
	scopes        string
	tokenEnv      string // base AGEZT_ env the access token is written to
	instanceBased bool
}

var oauthProviders = map[string]oauthProvider{
	"slack": {
		authURL:  "https://slack.com/oauth/v2/authorize",
		tokenURL: "https://slack.com/api/oauth.v2.access",
		scopes:   "chat:write,channels:read",
		tokenEnv: "AGEZT_SLACK_TOKEN",
	},
	"mastodon": {
		instanceBased: true,
		scopes:        "read write",
		tokenEnv:      "AGEZT_MASTODON_TOKEN",
	},
}

// oauthFlow is one in-flight authorization, keyed by its opaque state token.
type oauthFlow struct {
	kind         string
	label        string
	clientID     string
	clientSecret string
	redirectURI  string
	tokenURL     string
	tokenEnv     string
	status       string // "pending" | "done" | "error"
	errMsg       string
	created      time.Time
}

const oauthFlowTTL = 15 * time.Minute

// oauthClientFor builds the HTTP client for the token exchange. It defaults to an
// SSRF-guarded client (blocks loopback/private/link-local at dial — the token
// endpoint and any operator-supplied instance must be a public host); tests
// override it to reach an httptest server on loopback.
var oauthClientFor = func(timeout time.Duration) *http.Client {
	return netguard.New().HTTPClient(timeout)
}

// pruneOAuthLocked drops flows older than the TTL. Caller holds oauthMu.
func (s *Server) pruneOAuthLocked(now time.Time) {
	for k, f := range s.oauthPending {
		if now.Sub(f.created) > oauthFlowTTL {
			delete(s.oauthPending, k)
		}
	}
}

func newOAuthState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// handleChannelOAuthStart begins an OAuth flow: it records the client credentials
// + target account and returns the provider's authorize URL for the browser to
// open. args: kind, label, client_id, client_secret, redirect_uri, instance_url.
func (s *Server) handleChannelOAuthStart(conn net.Conn, req Request) {
	kind := strings.TrimSpace(strings.ToLower(stringArg(req.Args, "kind")))
	label := strings.TrimSpace(stringArg(req.Args, "label"))
	clientID := strings.TrimSpace(stringArg(req.Args, "client_id"))
	clientSecret := strings.TrimSpace(stringArg(req.Args, "client_secret"))
	redirectURI := strings.TrimSpace(stringArg(req.Args, "redirect_uri"))
	instanceURL := strings.TrimSpace(stringArg(req.Args, "instance_url"))

	prov, ok := oauthProviders[kind]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: kind + " does not support OAuth connect"})
		return
	}
	if label != "" && !settings.ValidAccountLabel(label) {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "label must be a slug: lowercase letters/digits/-/_, max 32"})
		return
	}
	if clientID == "" || clientSecret == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "client_id and client_secret are required"})
		return
	}
	if !isHTTPSURL(redirectURI) {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "redirect_uri must be an absolute http(s) URL"})
		return
	}

	authURL, tokenURL := prov.authURL, prov.tokenURL
	if prov.instanceBased {
		base, err := normalizeInstanceURL(instanceURL)
		if err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
		authURL = base + "/oauth/authorize"
		tokenURL = base + "/oauth/token"
	}

	state, err := newOAuthState()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "generate state: " + err.Error()})
		return
	}

	now := time.Now()
	s.oauthMu.Lock()
	if s.oauthPending == nil {
		s.oauthPending = map[string]*oauthFlow{}
	}
	s.pruneOAuthLocked(now)
	s.oauthPending[state] = &oauthFlow{
		kind: kind, label: label, clientID: clientID, clientSecret: clientSecret,
		redirectURI: redirectURI, tokenURL: tokenURL, tokenEnv: prov.tokenEnv,
		status: "pending", created: now,
	}
	s.oauthMu.Unlock()

	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", prov.scopes)
	q.Set("state", state)
	authorize := authURL + "?" + q.Encode()

	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"authorize_url": authorize, "state": state,
	}})
}

// handleChannelOAuthCallback completes a flow: it exchanges the code for an
// access token and writes it into the account's "#label" vault slot. Invoked by
// the daemon's public /oauth/callback HTTP handler. args: code, state.
//
// The ctx parameter is the connection-level context (cancelled when the
// control-plane connection closes, the client disconnects, or the daemon
// shuts down). We wrap it with a 20s upper bound so a stuck provider
// exchange cannot pin a worker indefinitely; the request-scoped context
// still propagates so a client disconnect or shutdown aborts the HTTP
// call promptly (BUG: dropped-r.Context, fixed by passing the bound ctx
// into exchangeOAuthCode instead of context.Background()).
func (s *Server) handleChannelOAuthCallback(ctx context.Context, conn net.Conn, req Request) {
	code := strings.TrimSpace(stringArg(req.Args, "code"))
	state := strings.TrimSpace(stringArg(req.Args, "state"))
	if code == "" || state == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "code and state required"})
		return
	}
	s.oauthMu.Lock()
	flow := s.oauthPending[state]
	s.oauthMu.Unlock()
	if flow == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown or expired state"})
		return
	}

	// Bound the exchange by the connection context + an upper bound so a
	// hung provider cannot pin the handler past the 20s mark AND client
	// cancellation / shutdown still aborts the call promptly.
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	token, err := s.exchangeOAuthCode(callCtx, flow, code)
	if err != nil {
		s.setOAuthStatus(state, "error", err.Error())
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "token exchange failed: " + err.Error()})
		return
	}

	key := settings.SuffixEnv(flow.tokenEnv, flow.label)
	vault := creds.NewStore(s.baseDir)
	if err := vault.Load(); err != nil {
		s.setOAuthStatus(state, "error", err.Error())
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "load vault: " + err.Error()})
		return
	}
	_ = vault.Set(key, token)
	if err := vault.Save(); err != nil {
		s.setOAuthStatus(state, "error", err.Error())
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save vault: " + err.Error()})
		return
	}
	s.setOAuthStatus(state, "done", "")
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"ok": true, "kind": flow.kind, "label": flow.label, "env": key, "applied": "restart",
	}})
}

// handleChannelOAuthStatus reports a flow's terminal state for the UI to poll.
// args: state.
func (s *Server) handleChannelOAuthStatus(conn net.Conn, req Request) {
	state := strings.TrimSpace(stringArg(req.Args, "state"))
	s.oauthMu.Lock()
	flow := s.oauthPending[state]
	s.oauthMu.Unlock()
	if flow == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"status": "unknown"}})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"status": flow.status, "error": flow.errMsg, "kind": flow.kind, "label": flow.label,
	}})
}

func (s *Server) setOAuthStatus(state, status, msg string) {
	s.oauthMu.Lock()
	if f := s.oauthPending[state]; f != nil {
		f.status = status
		f.errMsg = msg
	}
	s.oauthMu.Unlock()
}

// exchangeOAuthCode POSTs the authorization code to the provider's token endpoint
// over an SSRF-guarded client and extracts the access token. Provider responses
// vary: Slack signals failure with {"ok":false,"error":...}; all return the
// usable token at top-level "access_token".
func (s *Server) exchangeOAuthCode(ctx context.Context, flow *oauthFlow, code string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", flow.redirectURI)
	form.Set("client_id", flow.clientID)
	form.Set("client_secret", flow.clientSecret)

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, flow.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := oauthClientFor(20 * time.Second).Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var parsed struct {
		OK          *bool  `json:"ok"`
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("provider returned non-JSON (status %d)", resp.StatusCode)
	}
	if parsed.OK != nil && !*parsed.OK {
		return "", providerErr(parsed.Error, parsed.ErrorDesc, resp.StatusCode)
	}
	if parsed.AccessToken == "" {
		return "", providerErr(parsed.Error, parsed.ErrorDesc, resp.StatusCode)
	}
	return parsed.AccessToken, nil
}

func providerErr(code, desc string, status int) error {
	switch {
	case desc != "":
		return fmt.Errorf("%s", desc)
	case code != "":
		return fmt.Errorf("%s", code)
	default:
		return fmt.Errorf("no access_token in response (status %d)", status)
	}
}

// normalizeInstanceURL validates an operator-supplied instance URL (Mastodon)
// and returns its scheme://host[:port] base. SSRF to internal hosts is blocked
// at dial time by the netguard client during the token exchange; here we just
// enforce a well-formed https URL.
func normalizeInstanceURL(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("instance URL required for this provider")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid instance URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("instance URL must be http(s)")
	}
	return u.Scheme + "://" + u.Host, nil
}

func isHTTPSURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
