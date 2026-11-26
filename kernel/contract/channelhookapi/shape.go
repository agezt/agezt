// SPDX-License-Identifier: MIT

package channelhookapi

// The contract's own guard: the schema a member names against the Go type the
// daemon reads it as.
//
// The check is here — in the contract, the one home of the family's
// vocabulary — because the family's members are many packages, and a guard
// that each of them had to carry would be thirteen copies of the same
// question. A member asks the contract once, when it is built:
//
//	if err := channelhookapi.CheckShape(channelhookapi.MemberWebhook, reflect.TypeOf(inboundEnvelope{})); err != nil {
//	    panic("channels/webhook: the contract and the plugin disagree: " + err.Error())
//	}
//
// so a contract that mistyped a field stops the daemon instead of quietly
// disagreeing with the member that reads it.
//
// The expectation is the DAEMON's own type, not the schema's: a schema that
// mistyped a field would then agree with itself and pass.
//
// The comparison itself lives in the shared vocabulary (kernel/contract/
// jsonshape), because the ACP server surface (kernel/contract/acpserverapi)
// asks the very same question of its own schemas and must be answered the
// very same way.

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/agezt/agezt/kernel/contract/jsonshape"
)

// Fields returns the fields a member's body names, with the types the
// contract declares for them.
//
// It answers none for a member the contract does not know.
func Fields(memberID string) []jsonshape.Field {
	shape, ok := Body(memberID)
	if !ok {
		return nil
	}
	return jsonshape.Fields(shape)
}

// CheckShape compares a member's body schema with the Go type the daemon
// reads it as: the same fields, with the same types, in both directions.
//
// A field the daemon reads but the schema does not name is a hole in the
// door; a field the schema names but the daemon does not read is a promise the
// daemon cannot keep. Both are reported, and the report names them.
func CheckShape(memberID string, typ reflect.Type) error {
	shape, ok := Body(memberID)
	if !ok || len(shape) == 0 {
		return fmt.Errorf("the contract names no body for the %q member", memberID)
	}
	if len(Fields(memberID)) == 0 {
		return fmt.Errorf("the contract names no field for the %q member", memberID)
	}
	return checkShapeAgainst(shape, typ)
}

// checkShapeAgainst compares a body schema with the Go type the daemon reads
// it as — the same comparison, for a schema that is not in the table.
func checkShapeAgainst(shape json.RawMessage, typ reflect.Type) error {
	return jsonshape.CheckAgainst(shape, typ)
}
