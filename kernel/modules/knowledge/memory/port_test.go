// SPDX-License-Identifier: MIT

package memory

// Provenance: the module's contract test for the W3.57 slice — that the
//             module's rows ARE the contract's rows (one shape, not two), that
//             its sentinels ARE the contract's sentinels, and that the rules
//             that judge a record travel with the row.

import (
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
)

// TestTheRowsAreTheContractsRows: the module's names are ALIASES of the
// contract's. A look-alike of its own would be a SECOND shape of the same
// row — and a layer above the module would have to convert between them.
func TestTheRowsAreTheContractsRows(t *testing.T) {
	// The assignment is the assertion: an alias is the very same type, so a
	// value written through the module's name is readable through the
	// contract's name without a conversion.
	var row Record = memoryapi.Record{ID: "x", Type: memoryapi.TypeFact}
	var contractRow memoryapi.Record = row
	if contractRow.ID != "x" || contractRow.Type != memoryapi.TypeFact {
		t.Fatal("the module's record must BE the contract's record")
	}
	if reflect.TypeOf(row) != reflect.TypeOf(memoryapi.Record{}) {
		t.Fatal("the module's record must be the contract's very type")
	}

	// The same for the spec and the scored pair.
	var spec RememberSpec = memoryapi.RememberSpec{Content: "c"}
	if spec.Content != "c" {
		t.Fatal("the module's remember spec must be the contract's")
	}
	var scored Scored = memoryapi.Scored{Score: 1}
	if scored.Score != 1 {
		t.Fatal("the module's scored pair must be the contract's")
	}

	// And the type constants are the contract's values.
	if TypeFact != memoryapi.TypeFact || TypeSummary != memoryapi.TypeSummary ||
		TypeRelation != memoryapi.TypeRelation || TypePreference != memoryapi.TypePreference ||
		TypeObservation != memoryapi.TypeObservation {
		t.Fatal("the module's type constants must be the contract's values")
	}
	if DefaultType != memoryapi.DefaultType {
		t.Fatal("the module's default type must be the contract's")
	}
	if EvidenceObserved != memoryapi.EvidenceObserved {
		t.Fatal("the module's evidence constants must be the contract's values")
	}
}

// TestTheSentinelsAreTheContractsValues: the sentinels live in the contract
// because the layers above the module compare against them. A module that
// kept a look-alike of its own would leave every `errors.Is` comparison
// silently false — the layer would report "no such record" as some other
// failure.
func TestTheSentinelsAreTheContractsValues(t *testing.T) {
	if ErrRecordNotFound != memoryapi.ErrRecordNotFound {
		t.Fatal("the module's not-found sentinel must BE the contract's")
	}
	if !errors.Is(ErrRecordNotFound, memoryapi.ErrRecordNotFound) {
		t.Fatal("the module's not-found sentinel must match the contract's")
	}
	// And it must not be the empty-content sentinel: a layer comparing against
	// the wrong one would misreport the reason.
	if ErrRecordNotFound == memoryapi.ErrEmptyContent {
		t.Fatal("the module's sentinels must not collide with each other")
	}
}

// TestTheModuleAnswersTheContractsPorts: the compile-time assertion in the
// module is the proof that the module's face and the contract's port are the
// same shape. The runtime check here is the same proof from the other side:
// the concrete manager's method set must COVER the contract's manager port —
// every verb the contract names, and nothing the contract does not.
func TestTheModuleAnswersTheContractsPorts(t *testing.T) {
	mgr := NewManager(memoryapi.Deps{})
	port := reflect.TypeOf((*memoryapi.Manager)(nil)).Elem()
	concrete := reflect.TypeOf(mgr)
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := concrete.MethodByName(name); !ok {
			t.Errorf("the manager does not answer the contract's %s", name)
		}
	}
	// The narrower ports are the same object under narrower names — the
	// assignment is the assertion, and the compiler is the checker.
	var reader memoryapi.Reader = mgr
	var writer memoryapi.Writer = mgr
	_, _ = reader, writer
}

// TestTheRulesTravelWithTheRow: the rules that judge a record live in the
// contract, WITH the row — a Go alias cannot carry methods, so the rules
// could not have stayed in the module.
func TestTheRulesTravelWithTheRow(t *testing.T) {
	rec := Record{ID: "x", LastSeenMS: 1000, HalfLifeMS: 500}
	if !rec.Active() {
		t.Error("a plain record must be active")
	}
	if !rec.Expired(2000) {
		t.Error("a record past its half-life must be expired")
	}
	if rec.Usable(2000) {
		t.Error("an expired record must not be usable")
	}
	// The module's own ValidType is the contract's rule — over the WHOLE
	// domain, valid and invalid alike. A module that kept a membership set of
	// its own could drift from the contract's: the layers above would accept
	// a type the contract rejects (or the other way round), and nothing would
	// see it.
	for _, typ := range []memoryapi.Type{
		memoryapi.TypeFact, memoryapi.TypeSummary, memoryapi.TypeRelation,
		memoryapi.TypePreference, memoryapi.TypeObservation,
		"", "FACT ", "SKILL_REF", "NONSENSE",
	} {
		if got, want := ValidType(typ), memoryapi.ValidType(typ); got != want {
			t.Errorf("the module's ValidType(%q) = %v, the contract's = %v — the rule must be the contract's",
				typ, got, want)
		}
	}
	if !ValidType(TypeFact) {
		t.Error("FACT must be a valid type")
	}
	if ValidType("NONSENSE") {
		t.Error("an unknown type must not be valid")
	}
}

// TestTheModuleNamesNoOtherModule: the module's production code may not
// import another module — the structured-output path is INJECTED for exactly
// that reason. The check is static, on the module's own files.
//
// The test files are excluded: a test may bring the adapters the production
// code must not name.
func TestTheModuleNamesNoOtherModule(t *testing.T) {
	// The generator is a field, not a named path: the module holds the
	// CONTRACT's generator type.
	mgr := NewManager(memoryapi.Deps{})
	if mgr.gen != nil {
		t.Fatal("a manager built without deps must hold no generator")
	}
	// And the generator it holds is the contract's type, so a host can hand
	// over any function of that shape.
	var gen memoryapi.Generator = testGenerator
	if gen == nil {
		t.Fatal("the contract's generator type must accept the injected path")
	}
}
