// SPDX-License-Identifier: MIT
package opapi

import "context"

// TerminalWrite owns callbacks until the terminal response writer returns,
// including a returned write error. A writer panic discards them. After rejects
// closed ownership; false leaves the caller responsible for callback timing.
type TerminalWrite interface{ After(func()) bool }
type terminalWriteKey struct{}

func WithTerminalWrite(ctx context.Context, owner TerminalWrite) context.Context {
	return context.WithValue(ctx, terminalWriteKey{}, owner)
}
func AfterTerminalWrite(ctx context.Context, callback func()) bool {
	if callback == nil {
		return false
	}
	owner, ok := ctx.Value(terminalWriteKey{}).(TerminalWrite)
	return ok && owner != nil && owner.After(callback)
}
