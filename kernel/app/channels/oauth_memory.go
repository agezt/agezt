// SPDX-License-Identifier: MIT
package channels

import (
	"sync"
	"time"
)

const OAuthFlowTTL = 15 * time.Minute

var oauthProviders = map[string]OAuthProvider{
	"slack":    {AuthURL: "https://slack.com/oauth/v2/authorize", TokenURL: "https://slack.com/api/oauth.v2.access", Scopes: "chat:write,channels:read", TokenEnv: "AGEZT_SLACK_TOKEN"},
	"mastodon": {InstanceBased: true, Scopes: "read write", TokenEnv: "AGEZT_MASTODON_TOKEN"},
}

// OAuthMemory owns one host's in-flight flows. Flow returns value snapshots.
type OAuthMemory struct {
	mu        sync.Mutex
	pending   map[string]OAuthFlow
	clientFor func(time.Duration) OAuthHTTPClient
	vaultFor  func() AccountStore
}

func NewOAuthMemory(clientFor func(time.Duration) OAuthHTTPClient, vaultFor func() AccountStore) *OAuthMemory {
	return &OAuthMemory{clientFor: clientFor, vaultFor: vaultFor}
}
func (*OAuthMemory) Provider(kind string) (OAuthProvider, bool) {
	p, ok := oauthProviders[kind]
	return p, ok
}
func (*OAuthMemory) NewState() (string, error) { return NewOAuthState() }
func (*OAuthMemory) Now() time.Time            { return time.Now() }
func (p *OAuthMemory) Put(state string, flow OAuthFlow, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending == nil {
		p.pending = map[string]OAuthFlow{}
	}
	for key, candidate := range p.pending {
		if now.Sub(candidate.Created) > OAuthFlowTTL {
			delete(p.pending, key)
		}
	}
	p.pending[state] = flow
}
func (p *OAuthMemory) Flow(state string) (OAuthFlow, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	flow, ok := p.pending[state]
	return flow, ok
}
func (p *OAuthMemory) SetStatus(state, status, msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if flow, ok := p.pending[state]; ok {
		flow.Status = status
		flow.Error = msg
		p.pending[state] = flow
	}
}
func (p *OAuthMemory) Vault() AccountStore { return p.vaultFor() }
