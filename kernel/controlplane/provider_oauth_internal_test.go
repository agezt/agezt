// SPDX-License-Identifier: MIT

package controlplane

import "testing"

func TestProviderOAuthStateIsOwnedPerServer(t *testing.T) {
	a := NewServer(nil, t.TempDir())
	b := NewServer(nil, t.TempDir())
	first, repeated := a.providerOAuth(), a.providerOAuth()
	if first != repeated {
		t.Fatal("OAuth state recreated per request")
	}
	if a.providerOAuth() == b.providerOAuth() {
		t.Fatal("OAuth state shared between servers")
	}
}
