// SPDX-License-Identifier: MIT

package catalogapi

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// TestTheStorePortIsExactlyFiveVerbsWide: the store port is what the layers
// ABOVE the module drive — five verbs, no more.
//
// The width is pinned because a WIDER port would let a caller reach past the
// library's own rules (the store owns the merge order, the precedence and the
// sidecar), and a NARROWER one would force a second shape to be kept in step
// with this one. The module's own store has more (SaveCustom, SaveMeta);
// those are not part of the contract, because no layer above the module
// drives them.
func TestTheStorePortIsExactlyFiveVerbsWide(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Store must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 5 {
		t.Fatalf("the port has %d methods, want 5", typ.NumMethod())
	}
	for _, name := range []string{"Load", "SaveAPI", "SaveLocal", "UpsertCustomProvider", "LoadMeta"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have a %s method", name)
		}
	}
}

// TestTheStorePortNamesOnlyContractRows: every type the port names must be
// the contract's row or a builtin. A module's type in a port's signature
// would drag that module into every layer that holds the port.
func TestTheStorePortNamesOnlyContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	catalog := reflect.TypeOf((*Catalog)(nil))
	provider := reflect.TypeOf((*Provider)(nil))
	meta := reflect.TypeOf(Meta{})
	errType := reflect.TypeOf((*error)(nil)).Elem()
	str := reflect.TypeOf("")
	boolean := reflect.TypeOf(false)
	bytes := reflect.TypeOf([]byte(nil))

	load, _ := typ.MethodByName("Load")
	// Load() (*Catalog, error)
	if load.Type.NumIn() != 0 || load.Type.NumOut() != 2 || load.Type.Out(0) != catalog || load.Type.Out(1) != errType {
		t.Errorf("Load must answer (*Catalog, error), got %v", load.Type)
	}

	saveAPI, _ := typ.MethodByName("SaveAPI")
	// SaveAPI(raw []byte, sourceURL string) error
	if saveAPI.Type.NumIn() != 2 || saveAPI.Type.In(0) != bytes || saveAPI.Type.In(1) != str {
		t.Errorf("SaveAPI must take ([]byte, string), got %v", saveAPI.Type)
	}
	if saveAPI.Type.NumOut() != 1 || saveAPI.Type.Out(0) != errType {
		t.Errorf("SaveAPI must answer an error, got %v", saveAPI.Type)
	}

	saveLocal, _ := typ.MethodByName("SaveLocal")
	// SaveLocal(c *Catalog, source string) error
	if saveLocal.Type.NumIn() != 2 || saveLocal.Type.In(0) != catalog || saveLocal.Type.In(1) != str {
		t.Errorf("SaveLocal must take (*Catalog, string), got %v", saveLocal.Type)
	}
	if saveLocal.Type.NumOut() != 1 || saveLocal.Type.Out(0) != errType {
		t.Errorf("SaveLocal must answer an error, got %v", saveLocal.Type)
	}

	upsert, _ := typ.MethodByName("UpsertCustomProvider")
	// UpsertCustomProvider(p *Provider) (added bool, err error)
	if upsert.Type.NumIn() != 1 || upsert.Type.In(0) != provider {
		t.Errorf("UpsertCustomProvider must take a *Provider, got %v", upsert.Type)
	}
	if upsert.Type.NumOut() != 2 || upsert.Type.Out(0) != boolean || upsert.Type.Out(1) != errType {
		t.Errorf("UpsertCustomProvider must answer (bool, error), got %v", upsert.Type)
	}

	loadMeta, _ := typ.MethodByName("LoadMeta")
	// LoadMeta() (Meta, error)
	if loadMeta.Type.NumIn() != 0 || loadMeta.Type.NumOut() != 2 || loadMeta.Type.Out(0) != meta || loadMeta.Type.Out(1) != errType {
		t.Errorf("LoadMeta must answer (Meta, error), got %v", loadMeta.Type)
	}
}

// TestTheSyncerPortIsOneVerbAndTakesTheCallersURL: the syncer is the
// DAEMON's (W3.51) and the URL and the timeout are the CALLER's.
//
// They are parameters, not fields: a port cannot carry mutable state that a
// caller would have to reach through, and a caller that sets a URL on a port
// it does not own is reaching into someone else's implementation.
func TestTheSyncerPortIsOneVerbAndTakesTheCallersURL(t *testing.T) {
	typ := reflect.TypeOf((*Syncer)(nil)).Elem()
	if typ.NumMethod() != 1 {
		t.Fatalf("the syncer port has %d methods, want 1", typ.NumMethod())
	}
	sync, ok := typ.MethodByName("Sync")
	if !ok {
		t.Fatal("the syncer port must have a Sync method")
	}
	ctx := reflect.TypeOf((*context.Context)(nil)).Elem()
	str := reflect.TypeOf("")
	duration := reflect.TypeOf(time.Duration(0))
	catalog := reflect.TypeOf((*Catalog)(nil))
	errType := reflect.TypeOf((*error)(nil)).Elem()
	bytes := reflect.TypeOf([]byte(nil))

	// Sync(ctx context.Context, url string, timeout time.Duration) (raw []byte, cat *Catalog, res SyncResult, err error)
	if sync.Type.NumIn() != 3 || sync.Type.In(0) != ctx || sync.Type.In(1) != str || sync.Type.In(2) != duration {
		t.Errorf("Sync must take (context.Context, string, time.Duration), got %v", sync.Type)
	}
	if sync.Type.NumOut() != 4 || sync.Type.Out(0) != bytes || sync.Type.Out(1) != catalog || sync.Type.Out(3) != errType {
		t.Errorf("Sync must answer ([]byte, *Catalog, SyncResult, error), got %v", sync.Type)
	}
}

// TestTheDefaultSyncURLIsTheContracts: the source the catalog syncs from is
// named by the layers above the module, so it must be the contract's.
func TestTheDefaultSyncURLIsTheContracts(t *testing.T) {
	if DefaultSyncURL != "https://models.dev/api.json" {
		t.Fatalf("the default sync URL is %q", DefaultSyncURL)
	}
}

// TestTheCatalogRowCarriesItsRules: the row and the rules about it live
// together (W3.51). A rule that stayed in the module could not be reached by
// the layers above it — and the rules decide which model a run routes to.
func TestTheCatalogRowCarriesItsRules(t *testing.T) {
	cat := NewEmpty()
	if cat == nil || cat.Providers == nil {
		t.Fatal("the empty catalog must have a providers map")
	}
	cat.Providers["p"] = &Provider{ID: "p", Models: map[string]*Model{
		"m": {ID: "m", ToolCall: true, Limit: Limit{Context: 100}},
	}}
	prov, model := cat.FindModel("p/m")
	if prov == nil || model == nil {
		t.Fatal("the contract's row must find its own model")
	}
	if _, model := cat.FindModel("m"); model == nil {
		t.Fatal("a bare model id must resolve too")
	}
	// The catalog KNOWS the model, so the answer is known=true and
	// native=false — a missing capability, not a missing model.
	if native, known := cat.StrictToolArgsNative("m"); !known || native {
		t.Fatalf("StrictToolArgsNative on a non-native model = %v/%v, want false/true", native, known)
	}
	if len(cat.ProviderList()) != 1 {
		t.Fatalf("ProviderList returned %d providers", len(cat.ProviderList()))
	}
	if got := cat.DuplicateCredentialEnvs(); len(got) != 0 {
		t.Fatalf("DuplicateCredentialEnvs = %v, want empty", got)
	}
}
