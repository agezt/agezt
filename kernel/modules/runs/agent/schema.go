// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// ValidateJSON forwards the unchanged structured-output validation contract.
func ValidateJSON(def, value json.RawMessage) error {
	return schema.ValidateJSON(def, value)
}

// LintToolSchema forwards the unchanged registration-time schema lint.
func LintToolSchema(def toolapi.ToolDef) error {
	return schema.LintToolSchema(def)
}
