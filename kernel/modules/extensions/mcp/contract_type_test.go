// SPDX-License-Identifier: MIT

package mcp

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
)

// The module's Server IS the contract's: the tools and the marketplace that
// name it are PLUGINS, and a plugin may only reach the contract. Asserted as
// a function type — an assignment would also accept a copy with the same
// fields, while a function type requires identity.
var _ func(mcpapi.Server) = func(Server) {}

// The rule travels with the row: the module's Validate is the contract's
// Validate, not a second copy of it.
func TestValidateIsTheContractsValidate(t *testing.T) {
	cases := []Server{
		{Name: "github", Command: "npx"},
		{Name: "GitHub", Command: "npx"},
		{Name: "x", Command: "npx", URL: "https://a.invalid"},
		{Name: "x"},
		{Name: "x", Command: "npx", Args: []string{" "}},
	}
	for _, srv := range cases {
		modErr := Validate(srv)
		apiErr := mcpapi.Validate(srv)
		if (modErr == nil) != (apiErr == nil) {
			t.Fatalf("the module and the contract disagree on %+v: %v vs %v", srv, modErr, apiErr)
		}
		if modErr != nil && apiErr != nil && modErr.Error() != apiErr.Error() {
			t.Fatalf("the module and the contract disagree on the message for %+v: %q vs %q", srv, modErr, apiErr)
		}
	}
}
