// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/settings"
)

type OAuthProvider struct {
	AuthURL, TokenURL, Scopes, TokenEnv string
	InstanceBased                       bool
}
type OAuthFlow struct {
	Kind, Label, ClientID, ClientSecret, RedirectURI, TokenURL, TokenEnv, Status, Error string
	Created                                                                             time.Time
}
type OAuthPort interface {
	Provider(string) (OAuthProvider, bool)
	NewState() (string, error)
	Now() time.Time
	Put(string, OAuthFlow, time.Time)
	Flow(string) (OAuthFlow, bool)
	SetStatus(string, string, string)
	Exchange(context.Context, OAuthFlow, string) (string, error)
	Vault() AccountStore
}
type OAuth struct{ port OAuthPort }

func NewOAuth(port OAuthPort) *OAuth { return &OAuth{port: port} }

type OAuthStartInput struct{ Kind, Label, ClientID, ClientSecret, RedirectURI, InstanceURL string }
type OAuthCallbackInput struct{ Code, State string }
type OAuthStatusInput struct{ State string }
type OAuthStartOutput struct {
	AuthorizeURL string `json:"authorize_url"`
	State        string `json:"state"`
}
type OAuthCallbackOutput struct {
	OK      bool   `json:"ok"`
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Env     string `json:"env"`
	Applied string `json:"applied"`
}
type OAuthStatusOutput struct {
	Status string  `json:"status"`
	Error  *string `json:"error,omitempty"`
	Kind   *string `json:"kind,omitempty"`
	Label  *string `json:"label,omitempty"`
}

func (s *OAuth) Start(_ context.Context, in OAuthStartInput) (OAuthStartOutput, error) {
	kind := strings.TrimSpace(strings.ToLower(in.Kind))
	label := strings.TrimSpace(in.Label)
	clientID := strings.TrimSpace(in.ClientID)
	clientSecret := strings.TrimSpace(in.ClientSecret)
	redirectURI := strings.TrimSpace(in.RedirectURI)
	instanceURL := strings.TrimSpace(in.InstanceURL)
	provider, ok := s.port.Provider(kind)
	if !ok {
		return OAuthStartOutput{}, fmt.Errorf("%s does not support OAuth connect", kind)
	}
	if label != "" && !settings.ValidAccountLabel(label) {
		return OAuthStartOutput{}, fmt.Errorf("label must be a slug: lowercase letters/digits/-/_, max 32")
	}
	if clientID == "" || clientSecret == "" {
		return OAuthStartOutput{}, fmt.Errorf("client_id and client_secret are required")
	}
	if !IsHTTPSURL(redirectURI) {
		return OAuthStartOutput{}, fmt.Errorf("redirect_uri must be an https:// URL (http is allowed only for loopback dev: localhost, 127.0.0.1, ::1)")
	}
	authURL, tokenURL := provider.AuthURL, provider.TokenURL
	if provider.InstanceBased {
		base, err := NormalizeInstanceURL(instanceURL)
		if err != nil {
			return OAuthStartOutput{}, err
		}
		authURL = base + "/oauth/authorize"
		tokenURL = base + "/oauth/token"
	}
	state, err := s.port.NewState()
	if err != nil {
		return OAuthStartOutput{}, fmt.Errorf("generate state: %s", err)
	}
	now := s.port.Now()
	s.port.Put(state, OAuthFlow{Kind: kind, Label: label, ClientID: clientID, ClientSecret: clientSecret, RedirectURI: redirectURI, TokenURL: tokenURL, TokenEnv: provider.TokenEnv, Status: "pending", Created: now}, now)
	query := url.Values{}
	query.Set("client_id", clientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", provider.Scopes)
	query.Set("state", state)
	return OAuthStartOutput{AuthorizeURL: authURL + "?" + query.Encode(), State: state}, nil
}

func (s *OAuth) Callback(ctx context.Context, in OAuthCallbackInput) (OAuthCallbackOutput, error) {
	code := strings.TrimSpace(in.Code)
	state := strings.TrimSpace(in.State)
	if code == "" || state == "" {
		return OAuthCallbackOutput{}, fmt.Errorf("code and state required")
	}
	flow, ok := s.port.Flow(state)
	if !ok {
		return OAuthCallbackOutput{}, fmt.Errorf("unknown or expired state")
	}
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	token, err := s.port.Exchange(callCtx, flow, code)
	if err != nil {
		s.port.SetStatus(state, "error", err.Error())
		return OAuthCallbackOutput{}, fmt.Errorf("token exchange failed: %s", err)
	}
	key := settings.SuffixEnv(flow.TokenEnv, flow.Label)
	vault := s.port.Vault()
	if err := vault.Load(); err != nil {
		s.port.SetStatus(state, "error", err.Error())
		return OAuthCallbackOutput{}, fmt.Errorf("load vault: %s", err)
	}
	vault.Set(key, token)
	if err := vault.Save(); err != nil {
		s.port.SetStatus(state, "error", err.Error())
		return OAuthCallbackOutput{}, fmt.Errorf("save vault: %s", err)
	}
	s.port.SetStatus(state, "done", "")
	return OAuthCallbackOutput{OK: true, Kind: flow.Kind, Label: flow.Label, Env: key, Applied: "restart"}, nil
}

func (s *OAuth) Status(_ context.Context, in OAuthStatusInput) (OAuthStatusOutput, error) {
	flow, ok := s.port.Flow(strings.TrimSpace(in.State))
	if !ok {
		return OAuthStatusOutput{Status: "unknown"}, nil
	}
	return OAuthStatusOutput{Status: flow.Status, Error: &flow.Error, Kind: &flow.Kind, Label: &flow.Label}, nil
}
