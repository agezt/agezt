// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/platform/schema"
)

// ValidateToolInput forwards the unchanged tool schema validation contract.
func ValidateToolInput(def ToolDef, input json.RawMessage) error {
	return schema.ValidateToolInput(def, input)
}

// ValidateJSON forwards the unchanged structured-output validation contract.
func ValidateJSON(def, value json.RawMessage) error {
	return schema.ValidateJSON(def, value)
}

// LintToolSchema forwards the unchanged registration-time schema lint.
func LintToolSchema(def ToolDef) error {
	return schema.LintToolSchema(def)
}
