// SPDX-License-Identifier: MIT

package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
	"testing"
)

func TestProviderOAuthStateIsOwnedPerServer(t *testing.T) {
	a := NewServer(binding.ScopeFromKernel(nil, t.TempDir()))
	b := NewServer(binding.ScopeFromKernel(nil, t.TempDir()))
	first, repeated := a.providerOAuth(), a.providerOAuth()
	if first != repeated {
		t.Fatal("OAuth state recreated per request")
	}
	if a.providerOAuth() == b.providerOAuth() {
		t.Fatal("OAuth state shared between servers")
	}
}
