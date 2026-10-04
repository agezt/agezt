// SPDX-License-Identifier: MIT

package opapi

import "context"

type correlationKey struct{}

// WithCorrelation binds the host-owned operation identity for domain events.
func WithCorrelation(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey{}, id)
}

func CorrelationFromContext(ctx context.Context) string {
	id, _ := ctx.Value(correlationKey{}).(string)
	return id
}
