// SPDX-License-Identifier: MIT
package tools

import "github.com/agezt/agezt/kernel/toolforge"

// ForgeItem is the light native view; code stays in the detail view only.
type ForgeItem struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Language    string           `json:"language"`
	InputSchema string           `json:"input_schema,omitempty"`
	Status      toolforge.Status `json:"status"`
	TestedOK    bool             `json:"tested_ok"`
	TestedMS    int64            `json:"tested_ms,omitempty"`
	CreatedMS   int64            `json:"created_ms"`
	UpdatedMS   int64            `json:"updated_ms"`
	CallableAs  string           `json:"callable_as,omitempty"`
}

// ForgeDetail always includes code, including its empty string value.
type ForgeDetail struct {
	ForgeItem
	Code string `json:"code"`
}
