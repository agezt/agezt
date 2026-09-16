// SPDX-License-Identifier: MIT
//
// kernel/delegation depth-tracking context helpers (DepthFromCtx, WithDepth).
// Extracted from delegation.go during Day 211 god-file refactor (#90).
// Public API unchanged.
package delegation

import (
	"context"
)

func DepthFromCtx(ctx context.Context) int {
	if v, ok := ctx.Value(DepthKey{}).(int); ok {
		return v
	}
	return 0
}
func WithDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, DepthKey{}, depth)
}
