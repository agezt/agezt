// SPDX-License-Identifier: MIT

// Package apperrors provides standardized error wrapping and error code
// conventions for the Agezt kernel packages.
//
// Error Convention:
//
//   - Use [Wrap] to add context to an error with a error code prefix.
//     All kernel errors should use the package prefix as the error code.
//     Example: Wrap(ctx, "controlplane: listen", err) -> "controlplane: listen: <original>"
//
//   - Use [Wrapf] for formatted context messages.
//     Example: Wrapf(ctx, "agent: provider %s", name, err) -> "agent: provider anthropic: <original>"
//
//   - Sentinel errors should be defined at package level using errors.New.
//     Example: var ErrNotFound = errors.New("controlplane: not found")
//
//   - Validation errors should return early without wrapping.
//     Example: if x == nil { return ErrInvalidInput }
//
//   - For wrapped errors, always use %w verb to preserve error chain.
//     Example: fmt.Errorf("pkg: operation: %w", err)
package apperrors

import (
	"context"
	"fmt"
)

// Code represents a hierarchical error code namespace.
// Packages should define their own codes by extending this type.
type Code string

// Common error codes used across Agezt.
const (
	CodeUnknown       Code = "unknown"
	CodeInvalidInput  Code = "invalid_input"
	CodeNotFound      Code = "not_found"
	CodeAlreadyExists Code = "already_exists"
	CodeTimeout       Code = "timeout"
	CodePermission    Code = "permission"
	CodeInternal      Code = "internal"
	CodeExternal      Code = "external"
)

// Wrap wraps err with a prefix and returns a new error.
// If err is nil, nil is returned (no allocation).
//
// Usage:
//
//	Wrap(ctx, "controlplane: listen", err)
//
// The prefix should be in the format "package: operation".
// This follows the Go error convention of lowercase, colon-separated.
func Wrap(ctx context.Context, prefix string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

// WrapSimple is like Wrap but without context dependency.
// Use this when the calling function doesn't have a context parameter.
//
// Usage:
//
//	WrapSimple("controlplane: listen", err)
func WrapSimple(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

// Wrapf wraps err with a formatted prefix and returns a new error.
// If err is nil, nil is returned (no allocation).
//
// Usage:
//
//	Wrapf(ctx, "agent: provider %s: %w", providerName, err)
//
// Note: unlike fmt.Errorf, the error (%w) must be the LAST argument.
func Wrapf(ctx context.Context, format string, err error, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}

// WrapSimplef is like Wrapf but without context dependency.
// Use this when the calling function doesn't have a context parameter.
//
// Usage:
//
//	WrapSimplef("agent: provider %s: %w", providerName, err)
func WrapSimplef(format string, err error, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}
