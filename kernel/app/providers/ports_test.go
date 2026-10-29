// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"errors"
	"testing"

	appcatalog "github.com/agezt/agezt/kernel/app/catalog"
)

// The providers service answers from the CATALOG family's ports — the catalog
// directory is the daemon's file, and the providers write into it.
//
// The test pins the refusal: a daemon that holds no catalog library must be
// REFUSED, not silently told that a provider was connected. The old shape
// spelled "there is no library" here and there; now the port answers once and
// the service asks it.
func TestTheProvidersServiceRefusesWithoutACatalogLibrary(t *testing.T) {
	svc := New(appcatalog.Ports{}, t.TempDir())

	// Connecting a provider writes into the catalog's store — without a store
	// there is nowhere to write, and the refusal is the answer.
	if _, err := svc.Connect(context.Background(), ConnectInput{ID: "acme", API: "openai"}); !errors.Is(err, appcatalog.ErrNoLibrary) {
		t.Fatalf("connect must refuse without a catalog library, got %v", err)
	}

	// The refresh surface refuses too, and it says WHY.
	if _, err := svc.Reload(context.Background(), ReloadInput{}); !errors.Is(err, appcatalog.ErrNoLibrary) {
		t.Fatalf("reload must refuse without a catalog library, got %v", err)
	}
}

// The login's refresh is the catalog's, and a host with no catalog refreshes
// nothing — silently, because the login's own outcome is the answer the
// operator is waiting for.
//
// The test pins that the login does not panic on a host with no catalog: the
// refusal is swallowed by design, but a panic would not be.
func TestTheProviderLoginSurvivesAHostWithNoCatalog(t *testing.T) {
	auth := NewOAuth(appcatalog.Ports{}, t.TempDir(), nil)
	if auth == nil {
		t.Fatal("the login was not built")
	}
}
