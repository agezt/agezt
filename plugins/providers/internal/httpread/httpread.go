// SPDX-License-Identifier: MIT

// Package httpread provides a size-bounded reader for provider HTTP
// response bodies (M189). A provider talks to an endpoint that may be
// operator-configured to an arbitrary URL (the openai-compat / ollama /
// custom-base-URL path), buggy, or MITM'd. Reading the body with a plain
// io.ReadAll is unbounded, so a multi-gigabyte (or never-ending) response
// drives the daemon to OOM. All() caps the read and turns an over-size
// body into a clean error instead of memory exhaustion.
package httpread

import (
	"errors"
	"io"
)

// DefaultMaxResponseBytes caps a provider HTTP response body. LLM API
// replies — even large completions or tool-call argument blobs — are far
// under this; 64 MiB bounds a hostile or runaway endpoint without
// rejecting any legitimate response. It is a var (not a const) only so
// tests can lower it to drive the bound cheaply; providers read it
// synchronously when calling All, so a set-before-call test override is
// race-free.
var DefaultMaxResponseBytes int64 = 64 << 20

// ErrResponseTooLarge is returned by All when the body exceeds the cap.
var ErrResponseTooLarge = errors.New("provider: response body exceeds max size")

// All reads body fully but at most max bytes. If the body has more than
// max bytes it returns the first max bytes AND ErrResponseTooLarge, so a
// caller surfaces a clean error rather than letting the process OOM. A
// genuine read error from body is returned as-is. A max <= 0 uses
// DefaultMaxResponseBytes.
func All(body io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		max = DefaultMaxResponseBytes
	}
	b, err := io.ReadAll(io.LimitReader(body, max+1))
	if err != nil {
		return b, err
	}
	if int64(len(b)) > max {
		return b[:max], ErrResponseTooLarge
	}
	return b, nil
}
