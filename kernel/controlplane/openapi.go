// SPDX-License-Identifier: MIT

package controlplane

// The daemon's operation wire contracts, for the API document.
//
// The console's route catalogue (kernel/contract/opapi/http.go) says WHICH
// path answers WHICH operation and which arguments the console forwards; the
// operation registry says what each operation ACCEPTS and RETURNS. An OpenAPI
// document is the join of the two, so something has to read the registry.
//
// This is that something. The registry is reachable at BUILD time — the
// operations are bound in package initialisers, so no daemon, no socket and no
// running kernel is needed to ask what an operation's contract is. That is
// what makes the API document a derived artifact instead of a second copy:
// `go run ./tools/openapi` regenerates it from the same source the daemon
// serves from, and CI can check the committed file against it.

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

// AppOperationSchemas returns every registered operation's input and output
// schema, keyed by operation name.
//
// The map is a copy: the registry's schemas belong to the operations, and a
// caller that rewrote one would rewrite the daemon's own contract.
func AppOperationSchemas() map[string]opapi.OperationSchema {
	operations := registeredAppOperations()
	schemas := make(map[string]opapi.OperationSchema, len(operations))
	for _, operation := range operations {
		spec := operation.Spec()
		schemas[spec.Name] = opapi.OperationSchema{
			Input:  json.RawMessage(spec.InputSchema),
			Output: json.RawMessage(spec.OutputSchema),
		}
	}
	return schemas
}
