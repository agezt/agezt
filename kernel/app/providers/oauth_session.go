// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"errors"

	"github.com/agezt/agezt/kernel/chatgptauth"
)

var errProviderLoginExpired = errors.New("invalid or expired sign-in")

// providerLoginCurrentLocked requires provLoginMu and the captured login owner.
func (s *OAuth) providerLoginCurrentLocked(login *providerLogin) bool {
	if login == nil || s.provLogin != login {
		return false
	}
	select {
	case <-login.expiryStop:
		return false
	default:
		return true
	}
}

// persistProviderTokens serializes admission and storage with logout/retirement.
func (s *OAuth) persistProviderTokens(ctx context.Context, login *providerLogin, tokens chatgptauth.Tokens) error {
	s.provLoginMu.Lock()
	defer s.provLoginMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.providerLoginCurrentLocked(login) || login.status != "pending" {
		return errProviderLoginExpired
	}
	return s.chatgptMgr().StoreTokens(tokens)
}
