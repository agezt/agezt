// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"time"
)

type providerCallbackInput struct{ Code, State, Error string }
type providerCallbackResult struct {
	Success bool
	Message string
	Close   bool
}
type providerCodeExchange func(context.Context, string, string) error

// completeProviderLogin owns callback admission and effects independently of HTTP.
// The adapter renders its result and schedules the existing delayed listener close.
func (s *OAuth) completeProviderLogin(parent context.Context, login *providerLogin, in providerCallbackInput, exchange providerCodeExchange) providerCallbackResult {
	if in.Error != "" {
		s.setProviderLoginStatus(login, "error", "authorization denied: "+in.Error)
		return providerCallbackResult{Message: "Authorization was denied.", Close: true}
	}
	if in.Code == "" || in.State != login.state {
		return providerCallbackResult{Message: "Invalid or expired sign-in. Start again from the console."}
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if err := exchange(ctx, in.Code, login.verifier); err != nil {
		s.setProviderLoginStatus(login, "error", err.Error())
		return providerCallbackResult{Message: err.Error(), Close: true}
	}
	s.setProviderLoginStatus(login, "done", "")
	if s.k != nil {
		_, _, _ = s.k.Reload()
	}
	s.syncChatGPTModels()
	return providerCallbackResult{Success: true, Close: true}
}
