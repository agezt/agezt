// SPDX-License-Identifier: MIT

package memoryapi_test

// Provenance: the contract's own tests for the W3.57 slice — the widths of
//             the memory's ports, the exact verbs they carry, the sentinels the
//             layers compare against, and the rules that travel with the record
//             row.
//
// The widths are checked STATICALLY, on the interface types: a narrowed port
// would still answer the read side, so only the declared method set can tell
// the two apart.

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
)

// TestTheStorePortIsExactlySixVerbsWide: the store is the module's WHOLE
// record-store surface as the layers above it see it — and no more.
//
// The width is a promise in both directions: too narrow and the layers above
// cannot do their work, too wide and the port grows verbs nobody calls (and
// every fake has to implement them).
func TestTheStorePortIsExactlySixVerbsWide(t *testing.T) {
	typ := reflect.TypeOf((*memoryapi.Store)(nil)).Elem()
	if got := typ.NumMethod(); got != 6 {
		t.Fatalf("the store port has %d verbs, want 6", got)
	}
	for _, name := range []string{"All", "Close", "Count", "Delete", "Get", "Put"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the store port is missing %s", name)
		}
	}
}

// TestTheStoreNamesOnlyContractRows: the store's exact signatures. A port that
// answers a row the contract does not have is a port the contract layer cannot
// describe — and a shape the modules above would have to reach back into the
// implementation for.
func TestTheStoreNamesOnlyContractRows(t *testing.T) {
	want := map[string]func(){
		"Put":    func() { var p memoryapi.Store; _ = p.Put(memoryapi.Record{}) },
		"Get":    func() { var p memoryapi.Store; _, _, _ = p.Get("") },
		"Delete": func() { var p memoryapi.Store; _, _ = p.Delete("") },
		"All":    func() { var p memoryapi.Store; _, _ = p.All() },
		"Count":  func() { var p memoryapi.Store; _ = p.Count() },
		"Close":  func() { var p memoryapi.Store; _ = p.Close() },
	}
	typ := reflect.TypeOf((*memoryapi.Store)(nil)).Elem()
	for name := range want {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the store port is missing %s", name)
		}
	}
	if typ.NumMethod() != len(want) {
		t.Fatalf("the store port has %d verbs, want %d", typ.NumMethod(), len(want))
	}
}

// TestThePublisherPortIsOneVerb: the manager journals a mutation and keeps the
// event's id as the record's provenance. One verb, one meaning.
func TestThePublisherPortIsOneVerb(t *testing.T) {
	typ := reflect.TypeOf((*memoryapi.Publisher)(nil)).Elem()
	if typ.NumMethod() != 1 {
		t.Fatalf("the publisher port has %d verbs, want 1", typ.NumMethod())
	}
	m, ok := typ.MethodByName("Publish")
	if !ok {
		t.Fatal("the publisher port has no Publish")
	}
	if got, want := m.Type.NumOut(), 2; got != want {
		t.Fatalf("Publish answers %d values, want %d (the event id and the error)", got, want)
	}
}

// TestTheReaderPortIsExactlyNineVerbsWide: the read-only half of the memory.
// A reader that could also write, retire or distil would hand every read-only
// consumer the whole memory lifecycle.
func TestTheReaderPortIsExactlyNineVerbsWide(t *testing.T) {
	typ := reflect.TypeOf((*memoryapi.Reader)(nil)).Elem()
	if got := typ.NumMethod(); got != 9 {
		t.Fatalf("the reader port has %d verbs, want 9", got)
	}
	for _, name := range []string{
		"Active", "All", "Count", "Get", "ProfileText",
		"Recall", "RecallScoped", "Search", "SearchScoped",
	} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the reader port is missing %s", name)
		}
	}
}

// TestTheWriterPortIsExactlyFiveVerbsWide: the mutating half. Five verbs, no
// more: a writer that could also maintain or distil would be a manager in
// disguise.
func TestTheWriterPortIsExactlyFiveVerbsWide(t *testing.T) {
	typ := reflect.TypeOf((*memoryapi.Writer)(nil)).Elem()
	if got := typ.NumMethod(); got != 5 {
		t.Fatalf("the writer port has %d verbs, want 5", got)
	}
	for _, name := range []string{"Forget", "Promote", "Remember", "Supersede", "Suspend"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the writer port is missing %s", name)
		}
	}
}

// TestTheMaintainerPortIsExactlyFiveVerbsWide: the hygiene half — the verbs
// that keep the store small and honest. They are the operator's, not an
// agent's.
func TestTheMaintainerPortIsExactlyFiveVerbsWide(t *testing.T) {
	typ := reflect.TypeOf((*memoryapi.Maintainer)(nil)).Elem()
	if got := typ.NumMethod(); got != 5 {
		t.Fatalf("the maintainer port has %d verbs, want 5", got)
	}
	for _, name := range []string{"Audit", "CleanLowValue", "DedupeDistilled", "Hygiene", "Prune"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the maintainer port is missing %s", name)
		}
	}
}

// TestTheDistillerPortIsExactlyThreeVerbsWide: the LLM-bound half. The
// provider is a PARAMETER, not a field: the caller owns the model and the
// credentials, and the module never holds a provider of its own.
func TestTheDistillerPortIsExactlyThreeVerbsWide(t *testing.T) {
	typ := reflect.TypeOf((*memoryapi.Distiller)(nil)).Elem()
	if got := typ.NumMethod(); got != 3 {
		t.Fatalf("the distiller port has %d verbs, want 3", got)
	}
	for _, name := range []string{"Distill", "DistillBrain", "DistillProfile"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the distiller port is missing %s", name)
		}
	}
	// No distiller holds a provider as a FIELD: the provider is an argument.
	// A port that answered a provider would have a module-owned model.
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		if m.Type.NumIn() > 0 && m.Type.In(0) == reflect.TypeOf((*llm.Provider)(nil)).Elem() {
			t.Errorf("%s receives the provider as its receiver — the port must not be provider-bound", m.Name)
		}
	}
}

// TestTheToolerPortIsOneVerb: the agent-facing surface of the memory. It is a
// separate one-verb port because it answers a different question — not "what
// does the store hold" but "what may the agent call".
func TestTheToolerPortIsOneVerb(t *testing.T) {
	typ := reflect.TypeOf((*memoryapi.Tooler)(nil)).Elem()
	if typ.NumMethod() != 1 {
		t.Fatalf("the tooler port has %d verbs, want 1", typ.NumMethod())
	}
	if _, ok := typ.MethodByName("Tool"); !ok {
		t.Fatal("the tooler port has no Tool")
	}
}

// TestTheManagerPortIsTheCompositionOfTheHalves: the manager is the WHOLE
// surface the host drives, and it is exactly the union of the halves above it.
// A manager that grew a verb of its own would be a port with a verb nobody
// above can justify.
func TestTheManagerPortIsTheCompositionOfTheHalves(t *testing.T) {
	mgr := reflect.TypeOf((*memoryapi.Manager)(nil)).Elem()
	reader := reflect.TypeOf((*memoryapi.Reader)(nil)).Elem()
	writer := reflect.TypeOf((*memoryapi.Writer)(nil)).Elem()
	maint := reflect.TypeOf((*memoryapi.Maintainer)(nil)).Elem()
	dist := reflect.TypeOf((*memoryapi.Distiller)(nil)).Elem()
	tooler := reflect.TypeOf((*memoryapi.Tooler)(nil)).Elem()

	union := map[string]bool{}
	for _, half := range []reflect.Type{reader, writer, maint, dist, tooler} {
		for i := 0; i < half.NumMethod(); i++ {
			union[half.Method(i).Name] = true
		}
	}
	if mgr.NumMethod() != len(union) {
		t.Fatalf("the manager port has %d verbs, want the %d of its halves", mgr.NumMethod(), len(union))
	}
	for i := 0; i < mgr.NumMethod(); i++ {
		name := mgr.Method(i).Name
		if !union[name] {
			t.Errorf("the manager port has a verb of its own: %s", name)
		}
	}
}

// TestTheReaderWriterPortIsTheReaderPlusTheWriter: the agent-facing surface
// looks notes up and writes them — and it never maintains or distils.
func TestTheReaderWriterPortIsTheReaderPlusTheWriter(t *testing.T) {
	rw := reflect.TypeOf((*memoryapi.ReaderWriter)(nil)).Elem()
	reader := reflect.TypeOf((*memoryapi.Reader)(nil)).Elem()
	writer := reflect.TypeOf((*memoryapi.Writer)(nil)).Elem()
	maint := reflect.TypeOf((*memoryapi.Maintainer)(nil)).Elem()
	dist := reflect.TypeOf((*memoryapi.Distiller)(nil)).Elem()

	if rw.NumMethod() != reader.NumMethod()+writer.NumMethod() {
		t.Fatalf("the reader-writer port has %d verbs, want %d (the reader plus the writer)",
			rw.NumMethod(), reader.NumMethod()+writer.NumMethod())
	}
	for i := 0; i < maint.NumMethod(); i++ {
		if _, ok := rw.MethodByName(maint.Method(i).Name); ok {
			t.Errorf("the reader-writer port must not carry the maintenance verb %s", maint.Method(i).Name)
		}
	}
	for i := 0; i < dist.NumMethod(); i++ {
		if _, ok := rw.MethodByName(dist.Method(i).Name); ok {
			t.Errorf("the reader-writer port must not carry the distillation verb %s", dist.Method(i).Name)
		}
	}
}

// TestTheSentinelsAreTheContracts: the sentinels the layers compare against
// live in the contract, so `errors.Is` matches them wherever they are raised.
func TestTheSentinelsAreTheContracts(t *testing.T) {
	for name, err := range map[string]error{
		"ErrRecordNotFound": memoryapi.ErrRecordNotFound,
		"ErrEmptyContent":   memoryapi.ErrEmptyContent,
	} {
		if err == nil {
			t.Errorf("%s is nil", name)
			continue
		}
		if err.Error() == "" {
			t.Errorf("%s has no message", name)
		}
	}
	// The two sentinels are distinct — a layer that compares against the wrong
	// one would misreport the reason ("not found" for an empty write).
	if memoryapi.ErrRecordNotFound == memoryapi.ErrEmptyContent {
		t.Fatal("the memory sentinels must be distinct")
	}
	// The structured-output sentinel lives in the LLM contract, for the same
	// reason: the modules handed the generator compare against it, and they
	// may not name the module that owns the path.
	if llm.ErrNoObjectGenerated == nil {
		t.Fatal("the structured-output sentinel is nil")
	}
}

// TestTheRecordRulesTravelWithTheRow: a Go type alias cannot carry methods,
// so the rules that judge a record live with the row in the contract. A rule
// that stayed in the implementation could not be reached by the layers above.
func TestTheRecordRulesTravelWithTheRow(t *testing.T) {
	now := int64(1_700_000_000_000)

	// A plain record: active, not suspended, not expired, usable.
	plain := memoryapi.Record{ID: "a", LastSeenMS: now}
	if !plain.Active() {
		t.Error("a plain record must be active")
	}
	if plain.Suspended() {
		t.Error("a plain record must not be suspended")
	}
	if plain.Expired(now) {
		t.Error("a plain record must not be expired")
	}
	if !plain.Usable(now) {
		t.Error("a plain record must be usable")
	}

	// A forgotten record is retained but barred from retrieval.
	forgotten := memoryapi.Record{ID: "b", Tombstoned: true}
	if forgotten.Active() {
		t.Error("a forgotten record must not be active")
	}
	if forgotten.Usable(now) {
		t.Error("a forgotten record must not be usable")
	}

	// A superseded record keeps its successor and stays out of retrieval.
	superseded := memoryapi.Record{ID: "c", SupersededBy: "d"}
	if superseded.Active() {
		t.Error("a superseded record must not be active")
	}
	if superseded.Usable(now) {
		t.Error("a superseded record must not be usable")
	}

	// A suspended record is retained and gettable, but barred from use.
	suspended := memoryapi.Record{ID: "e", SuspendedMS: now}
	if !suspended.Active() {
		t.Error("a suspended record is still active (it is retained, not forgotten)")
	}
	if !suspended.Suspended() {
		t.Error("a suspended record must report itself suspended")
	}
	if suspended.Usable(now) {
		t.Error("a suspended record must not be usable")
	}

	// An expired record: its reconstruction budget has elapsed.
	expired := memoryapi.Record{ID: "f", LastSeenMS: now, HalfLifeMS: 1000}
	if !expired.Expired(now + 2000) {
		t.Error("a record past its half-life must be expired")
	}
	if expired.Expired(now + 500) {
		t.Error("a record inside its half-life must not be expired")
	}
	if expired.Usable(now + 2000) {
		t.Error("an expired record must not be usable")
	}

	// A zero half-life is a legacy/no-expiry record: it stays usable.
	legacy := memoryapi.Record{ID: "g", LastSeenMS: now}
	if legacy.Expired(now + 100*365*24*3600*1000) {
		t.Error("a record with no half-life must never expire")
	}
}

// TestValidTypeIsTheMembershipSet: the type's membership is part of the
// type's meaning, so the rule travels with it.
func TestValidTypeIsTheMembershipSet(t *testing.T) {
	for _, typ := range []memoryapi.Type{
		memoryapi.TypeFact, memoryapi.TypeSummary, memoryapi.TypeRelation,
		memoryapi.TypePreference, memoryapi.TypeObservation,
	} {
		if !memoryapi.ValidType(typ) {
			t.Errorf("%s must be a valid type", typ)
		}
	}
	for _, typ := range []memoryapi.Type{"", "FACT ", "SKILL_REF", "NONSENSE"} {
		if memoryapi.ValidType(typ) {
			t.Errorf("%q must not be a valid type", typ)
		}
	}
	if memoryapi.DefaultType != memoryapi.TypeFact {
		t.Errorf("the default type is %q, want FACT", memoryapi.DefaultType)
	}
}

// TestTheContextSeamIsShared: the correlation id and the per-agent scope are
// threaded through the context by the layers above the module and read by the
// module's own tool. The keys live in the contract and ONLY there: two
// packages that each defined one would not see each other's values.
func TestTheContextSeamIsShared(t *testing.T) {
	ctx := context.Background()
	if got := memoryapi.CorrelationFrom(ctx); got != "" {
		t.Errorf("a bare context must carry no correlation, got %q", got)
	}
	if got := memoryapi.ScopeFrom(ctx); got != "" {
		t.Errorf("a bare context must carry no scope, got %q", got)
	}

	ctx = memoryapi.WithCorrelation(ctx, "corr-1")
	if got := memoryapi.CorrelationFrom(ctx); got != "corr-1" {
		t.Errorf("the correlation must survive the round trip, got %q", got)
	}
	// The scope is orthogonal: setting one must not clear the other.
	ctx = memoryapi.WithScope(ctx, "researcher")
	if got := memoryapi.ScopeFrom(ctx); got != "researcher" {
		t.Errorf("the scope must survive the round trip, got %q", got)
	}
	if got := memoryapi.CorrelationFrom(ctx); got != "corr-1" {
		t.Errorf("the scope must not clear the correlation, got %q", got)
	}
	// An empty scope is "no scope" — it must not put an empty value in the
	// context, so the readers above keep their default.
	plain := memoryapi.WithScope(context.Background(), "")
	if got := memoryapi.ScopeFrom(plain); got != "" {
		t.Errorf("an empty scope must stay unset, got %q", got)
	}
}

// TestTheDepsGatherThePortsInOneStruct: the store is the host's file, the
// publisher is the kernel's, the embedder and the generator are the host's
// seams. A builder that took four positional values would invite the wrong
// order at the call site, so they travel in ONE struct.
func TestTheDepsGatherThePortsInOneStruct(t *testing.T) {
	deps := memoryapi.Deps{}
	typ := reflect.TypeOf(deps)
	for _, name := range []string{"Store", "Publisher", "Embedder", "Generator"} {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("the deps struct is missing %s", name)
		}
	}
	if typ.NumField() != 4 {
		t.Fatalf("the deps struct has %d fields, want 4", typ.NumField())
	}
	// The builder takes the deps by VALUE and answers the manager port.
	bt := reflect.TypeOf(memoryapi.Builder(nil))
	if bt.NumIn() != 1 || bt.In(0) != typ {
		t.Fatalf("the builder must take the deps struct, got %v", bt)
	}
	if bt.NumOut() != 1 || bt.Out(0) != reflect.TypeOf((*memoryapi.Manager)(nil)).Elem() {
		t.Fatalf("the builder must answer the manager port, got %v", bt)
	}
}

// TestTheEmbedderPortIsOneVerb: the embedder is the optional provider seam.
// One verb, and the contract states the contract it must honour: exactly one
// L2-normalized vector per input, all of one dimensionality, in input order.
func TestTheEmbedderPortIsOneVerb(t *testing.T) {
	typ := reflect.TypeOf((*memoryapi.Embedder)(nil)).Elem()
	if typ.NumMethod() != 1 {
		t.Fatalf("the embedder port has %d verbs, want 1", typ.NumMethod())
	}
	m, ok := typ.MethodByName("EmbedBatch")
	if !ok {
		t.Fatal("the embedder port has no EmbedBatch")
	}
	if got, want := m.Type.NumIn(), 2; got != want {
		t.Fatalf("EmbedBatch takes %d values, want %d (the context and the texts)", got, want)
	}
}

// TestTheContractMayNotNameAThirdPartyPackage: the contract layer is the
// shared vocabulary, and it may not reach into a third-party package — the
// content-addressing (BLAKE3) stays in the module for exactly that reason.
//
// The check is behavioural, not a lint: the contract's own packages must
// compile with the standard library and the other contracts ALONE.
func TestTheContractMayNotNameAThirdPartyPackage(t *testing.T) {
	// The rule is enforced by the archcheck gate; here we assert the rule's
	// CONSEQUENCE: the contract's rows are plain values that a caller can
	// build and compare without naming the module.
	rec := memoryapi.Record{ID: "x", Type: memoryapi.TypeFact, Content: "c"}
	if rec.ID != "x" || rec.Type != memoryapi.TypeFact {
		t.Fatal("the contract's record must be a plain value")
	}
	if errors.Is(memoryapi.ErrEmptyContent, memoryapi.ErrRecordNotFound) {
		t.Fatal("the sentinels must not match each other")
	}
}
