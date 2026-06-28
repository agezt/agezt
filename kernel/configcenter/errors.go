// SPDX-License-Identifier: MIT

package configcenter

import (
	"fmt"
)

// Error codes
const (
	ErrKeyNotFound   = "KEY_NOT_FOUND"
	ErrAccessDenied  = "ACCESS_DENIED"
	ErrRatingDenied  = "RATING_DENIED"
	ErrRateLimited   = "RATE_LIMITED"
	ErrValueChanged  = "VALUE_CHANGED"
	ErrUnknownPolicy = "UNKNOWN_POLICY"
	ErrInternal      = "INTERNAL_ERROR"
)

// ConfigError represents a config center error with code and message.
type ConfigError struct {
	Code    string
	Message string
	Extra   map[string]string // Additional context (e.g., old_hash, new_hash for VALUE_CHANGED)
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap allows errors.Is and errors.As to work.
func (e *ConfigError) Unwrap() error {
	return nil
}

// NewConfigError creates a new ConfigError.
func NewConfigError(code, message string) *ConfigError {
	return &ConfigError{Code: code, Message: message}
}

// WithExtra adds extra context to the error and returns it.
func (e *ConfigError) WithExtra(key, value string) *ConfigError {
	if e.Extra == nil {
		e.Extra = make(map[string]string)
	}
	e.Extra[key] = value
	return e
}
