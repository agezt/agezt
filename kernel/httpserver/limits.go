// SPDX-License-Identifier: MIT

package httpserver

import "net/http"

// BodyLimit applies net/http's streaming request-body cap. The wrapped handler
// retains responsibility for mapping *http.MaxBytesError into its own response
// envelope.
func BodyLimit(maxBytes int64) func(http.HandlerFunc) http.HandlerFunc {
	if maxBytes <= 0 {
		panic("httpserver: body limit must be positive")
	}
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next(w, r)
		}
	}
}
