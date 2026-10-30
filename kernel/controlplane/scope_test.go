// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"io"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app/binding"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// scopeForKernel is the scope of one kernel, for the tests that need a
// request's scope without a daemon around them.
func scopeForKernel(k *runtime.Kernel) binding.Scope {
	return binding.ScopeFromKernel(k, "")
}

// The adapter holds NO handle.
//
// The composition root holds the kernel; the adapter holds the scope — every
// family's ports, filled. A field that could hold anything could hold the
// kernel, and a handle in the adapter is a door back to every verb the port
// programme gave away.
//
// The rule is the reflection: the adapter's own types have no untyped field,
// and the other ratchet (the control plane names the runtime in no file)
// holds the other half — a handle can only be typed by naming the runtime.
func TestTheAdapterHoldsNoHandle(t *testing.T) {
	anyType := reflect.TypeOf((*any)(nil)).Elem()
	for _, owner := range []reflect.Type{
		reflect.TypeOf(Server{}),
		reflect.TypeOf(DispatchCtx{}),
		reflect.TypeOf(appHost{}),
	} {
		for i := 0; i < owner.NumField(); i++ {
			field := owner.Field(i)
			if field.Type == anyType {
				t.Errorf("%s.%s holds a handle — the adapter stores the scope, not the kernel behind it", owner.Name(), field.Name)
			}
		}
	}
}

// The scope the composition root built is the scope the adapter reads.
func TestTheServerReadsTheScopeItWasGiven(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	s := NewServer(binding.ScopeFromKernel(k, dir), dir)
	scope := s.scoped()
	if !scope.Filled() {
		t.Fatal("the server did not take the scope it was given")
	}
	if scope.BaseDir != dir {
		t.Errorf("the server's scope is %q, want %q", scope.BaseDir, dir)
	}
	if s.journalPorts().Events == nil {
		t.Error("the server's scope carries no journal")
	}
}

// A server built without a scope asks the refusing one: a family is asked,
// never called into nothing.
func TestTheServerWithoutAScopeRefuses(t *testing.T) {
	s := &Server{}
	scope := s.scoped()
	if scope.Filled() {
		t.Fatal("a server without a scope filled one")
	}
	if scope.FilesRunner == nil {
		t.Error("the refusing scope left the files' runner empty")
	}
	if scope.JournalPorts.Events != nil {
		t.Error("the refusing scope filled a family's port")
	}
}

// The tenant's scope is the TENANT's: its own directory, its own families.
//
// The tenant's kernel is opened in the application layer and what comes back
// is a scope — the adapter never holds the tenant's handle either.
func TestTheTenantsScopeIsTheTenantsOwn(t *testing.T) {
	dir := t.TempDir()
	primary, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { primary.Close() })

	s := NewServer(binding.ScopeFromKernel(primary, dir), dir)
	registry, err := tenant.New(filepath.Join(dir, "tenants"), func(_ string, baseDir string) (io.Closer, error) {
		return runtime.Open(runtime.Config{BaseDir: baseDir, Provider: mock.New()})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.CloseAll() })
	s.SetTenants(registry)
	entry, err := registry.Acquire("acme", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	scope, err := s.scopeFor("acme")
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Filled() {
		t.Fatal("the tenant's scope was not filled")
	}
	if scope.BaseDir != entry.BaseDir {
		t.Errorf("the tenant's scope is %q, want the tenant's own %q", scope.BaseDir, entry.BaseDir)
	}
	if scope.BaseDir == dir {
		t.Error("the tenant's scope is the primary's — the tenant is not isolated")
	}
	if scope.JournalPorts.Events == nil {
		t.Error("the tenant's scope carries no journal")
	}
}

// The primary's scope is the primary's, and a tenant's request never sees
// it.
func TestThePrimaryScopeIsThePrimarysOwn(t *testing.T) {
	dir := t.TempDir()
	primary, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { primary.Close() })

	s := NewServer(binding.ScopeFromKernel(primary, dir), dir)
	scope, err := s.scopeFor("")
	if err != nil {
		t.Fatal(err)
	}
	if scope.BaseDir != dir {
		t.Errorf("the primary's scope is %q, want %q", scope.BaseDir, dir)
	}
	if !scope.Filled() {
		t.Error("the primary's scope was not filled")
	}
}

// A daemon without a tenant registry says so, in the words it always said.
func TestTheScopeForATenantWithoutARegistryRefuses(t *testing.T) {
	s := &Server{}
	if _, err := s.scopeFor("acme"); err == nil {
		t.Fatal("a tenant was routed without a tenant registry")
	} else if err.Error() != "multi-tenancy is disabled (no tenant registry configured)" {
		t.Errorf("error = %v, want the daemon's own refusal", err)
	}
}

// The request carries the SCOPE: the host in the context is the scope of the
// daemon the request was routed to.
func TestTheRequestCarriesTheScopeAndNotTheHandle(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	scope := binding.ScopeFromKernel(k, dir)
	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scope})
	got := requestScope(ctx)
	if !got.Filled() || got.BaseDir != dir {
		t.Errorf("the request's scope is %q, want %q", got.BaseDir, dir)
	}
}
