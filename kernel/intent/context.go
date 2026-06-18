// SPDX-License-Identifier: MIT

package intent

import "context"

type frameCtxKey struct{}

// WithFrame carries an already-interpreted intent frame across package
// boundaries. Executors use it to avoid reinterpreting raw or derived text.
func WithFrame(ctx context.Context, frame Frame) context.Context {
	return context.WithValue(ctx, frameCtxKey{}, frame)
}

// FrameFromContext returns a frame previously attached with WithFrame.
func FrameFromContext(ctx context.Context) (Frame, bool) {
	frame, ok := ctx.Value(frameCtxKey{}).(Frame)
	return frame, ok
}
