// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/app/providers"
)

func (s *Server) providerOAuth() *providers.OAuth {
	s.providerOAuthOnce.Do(func() {
		s.providerOAuthState = providers.NewOAuth(s.k, s.baseDir, func() ([]string, string) {
			if s.chatgptSync == nil {
				return nil, ""
			}
			return s.chatgptSync()
		})
	})
	return s.providerOAuthState
}
