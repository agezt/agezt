// SPDX-License-Identifier: MIT

package providers

import "time"

func (s *OAuth) closeProviderLogin(login *providerLogin) {
	if login == nil {
		return
	}
	if login.expiryStop != nil {
		login.expiryOnce.Do(func() { close(login.expiryStop) })
	}
	if login.srv != nil {
		_ = login.srv.Close()
	}
}

func (s *OAuth) expireProviderLogin(login *providerLogin, ttl time.Duration) {
	defer close(login.expiryDone)
	timer := time.NewTimer(ttl)
	defer timer.Stop()
	select {
	case <-login.expiryStop:
		return
	case <-timer.C:
	}
	s.provLoginMu.Lock()
	select {
	case <-login.expiryStop:
		s.provLoginMu.Unlock()
		return
	default:
	}
	if s.provLogin != login {
		s.provLoginMu.Unlock()
		return
	}
	if login.status == "pending" {
		login.status = "error"
		login.errMsg = "sign-in timed out"
	}
	s.provLoginMu.Unlock()
	s.closeProviderLogin(login)
}
