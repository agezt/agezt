// SPDX-License-Identifier: MIT
package opapi

import "context"

// TerminalCleanup accepts ownership of a cleanup until terminal delivery ends.
// A false return leaves ownership with the caller. Transports own the scope.
type TerminalCleanup interface {
	Defer(func()) bool
}

type terminalCleanupKey struct{}

func WithTerminalCleanup(ctx context.Context, owner TerminalCleanup) context.Context {
	return context.WithValue(ctx, terminalCleanupKey{}, owner)
}

func DeferTerminalCleanup(ctx context.Context, cleanup func()) bool {
	if cleanup == nil {
		return false
	}
	owner, ok := ctx.Value(terminalCleanupKey{}).(TerminalCleanup)
	return ok && owner != nil && owner.Defer(cleanup)
}
