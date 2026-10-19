// SPDX-License-Identifier: MIT

package app

import "context"

// requestIDKey is the context key for the caller's request id.
type requestIDKey struct{}

// WithRequestID records the caller's request id in the context, so an
// operation that names something after the call — a plan after the request
// that submitted it — can find it.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the caller's request id, or "" when there is none.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
