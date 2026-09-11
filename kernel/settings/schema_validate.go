// SPDX-License-Identifier: MIT

// Settings validation: Validate.
// Code extracted from schema.go during the Day-62 god-file split. Public API unchanged.
package settings


import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)



// Validate checks a value against its field's type. Empty is always allowed
// (clearing a field). Returns nil for unknown fields the caller already rejected.
func Validate(f Field, value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	switch f.Type {
	case TypeNumber:
		if _, err := strconv.Atoi(v); err != nil {
			return fmt.Errorf("%s must be a whole number", f.Label)
		}
	case TypeBool:
		switch strings.ToLower(v) {
		case "1", "0", "true", "false", "on", "off", "yes", "no":
		default:
			return fmt.Errorf("%s must be a boolean (on/off, true/false, 1/0)", f.Label)
		}
	case TypeSelect:
		if !slices.Contains(f.Options, v) {
			return fmt.Errorf("%s must be one of: %s", f.Label, strings.Join(f.Options, ", "))
		}
	}
	return nil
}