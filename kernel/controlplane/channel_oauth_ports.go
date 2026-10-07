// SPDX-License-Identifier: MIT
package controlplane

import (
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/netguard"
	"net/http"
	"time"
)

// The guarded client factory remains a native selection point and test hook.
var oauthClientFor = func(timeout time.Duration) *http.Client { return netguard.New().HTTPClient(timeout) }

func (s *Server) channelOAuth() *appchannels.OAuth {
	s.channelOAuthOnce.Do(func() {
		s.channelOAuthState = appchannels.NewOAuthMemory(func(timeout time.Duration) appchannels.OAuthHTTPClient { return oauthClientFor(timeout) }, func() appchannels.AccountStore { return nativeChannelVault{creds.NewStore(s.baseDir)} })
		s.channelOAuthService = appchannels.NewOAuth(s.channelOAuthState)
	})
	return s.channelOAuthService
}
