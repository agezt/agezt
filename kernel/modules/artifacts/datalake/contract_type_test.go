// SPDX-License-Identifier: MIT

package datalake

import (
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/datalakeapi"
)

// TestTheRowsAreTheContractRows: every row the module keeps is the
// CONTRACT'S row, not a copy of it. A copy would drift — the module would
// then keep a shape the contract no longer describes, and the agent-facing
// tool (which speaks the contract) would no longer fit it.
func TestTheRowsAreTheContractRows(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   any
		contract any
	}{
		{"Field", Field{}, datalakeapi.Field{}},
		{"Schema", Schema{}, datalakeapi.Schema{}},
		{"Record", Record{}, datalakeapi.Record{}},
		{"Query", Query{}, datalakeapi.Query{}},
		{"CollectionInfo", CollectionInfo{}, datalakeapi.CollectionInfo{}},
	} {
		if reflect.TypeOf(c.module) != reflect.TypeOf(c.contract) {
			t.Errorf("the %s row drifted from the contract: %v vs %v",
				c.name, reflect.TypeOf(c.module), reflect.TypeOf(c.contract))
		}
	}
}

// TestTheErrorsAreTheContracts: the refusals the lake answers with are the
// contract's — the tool maps them to three different answers, so there must
// be one error and one spelling of it.
func TestTheErrorsAreTheContracts(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   error
		contract error
	}{
		{"ErrNotFound", ErrNotFound, datalakeapi.ErrNotFound},
		{"ErrExists", ErrExists, datalakeapi.ErrExists},
		{"ErrSystem", ErrSystem, datalakeapi.ErrSystem},
	} {
		if !errors.Is(c.module, c.contract) {
			t.Errorf("the %s error is not the contract's: %v vs %v", c.name, c.module, c.contract)
		}
		if c.module.Error() != c.contract.Error() {
			t.Errorf("the %s message differs: %q vs %q", c.name, c.module, c.contract)
		}
	}
}

// TestTheLakeIsTheContractsPort: the lake answers the contract's port, so
// the agent-facing tool can be handed it without ever naming this module.
func TestTheLakeIsTheContractsPort(t *testing.T) {
	var _ datalakeapi.Lake = (*Lake)(nil)
}
