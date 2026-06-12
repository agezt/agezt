// SPDX-License-Identifier: MIT

package configcenter

import (
	"errors"
	"fmt"
)

// Error codes
const (
	ErrKeyNotFound     = "KEY_NOT_FOUND"
	ErrAccessDenied    = "ACCESS_DENIED"
	ErrRatingDenied    = "RATING_DENIED"
	ErrRateLimited     = "RATE_LIMITED"
	ErrValueChanged    = "VALUE_CHANGED"
	ErrUnknownPolicy   = "UNKNOWN_POLICY"
	ErrInternal        = "INTERNAL_ERROR"
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

// NewConfigErrorf creates a new ConfigError with formatted message.
func NewConfigErrorf(code, format string, args ...any) *ConfigError {
	return &ConfigError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WithExtra adds extra context to the error and returns it.
func (e *ConfigError) WithExtra(key, value string) *ConfigError {
	if e.Extra == nil {
		e.Extra = make(map[string]string)
	}
	e.Extra[key] = value
	return e
}

// IsNotFound checks if the error is a key-not-found error.
func IsNotFound(err error) bool {
	var cerr *ConfigError
	if errors.As(err, &cerr) {
		return cerr.Code == ErrKeyNotFound
	}
	return false
}

// IsAccessDenied checks if the error is an access-denied error.
func IsAccessDenied(err error) bool {
	var cerr *ConfigError
	if errors.As(err, &cerr) {
		return cerr.Code == ErrAccessDenied || cerr.Code == ErrRatingDenied
	}
	return false
}

// IsRateLimited checks if the error is a rate-limit error.
func IsRateLimited(err error) bool {
	var cerr *ConfigError
	if errors.As(err, &cerr) {
		return cerr.Code == ErrRateLimited
	}
	return false
}
