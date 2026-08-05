// SPDX-License-Identifier: MIT

package retry

import (
	"context"
	"net/http"

	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

// DoHTTP executes an HTTP call with the package's transient-error retry
// (connection failures, 429/5xx with Retry-After honoured) and returns the
// bounded response body. build is called PER ATTEMPT so signed requests
// (SigV4, short-lived OAuth tokens) are re-built fresh each try; a build
// error is terminal. A non-2xx status after retries surfaces as *HTTPError —
// adapters map it onto their own APIError type.
//
// This is the one Complete-path transport for every provider adapter (LD-4):
// before it existed only openai/anthropic/ollama retried, so a Gemini or
// Bedrock 429 failed the run where the identical OpenAI 429 recovered.
func DoHTTP(ctx context.Context, client *http.Client, build func() (*http.Request, error), maxBytes int64) ([]byte, http.Header, error) {
	if client == nil {
		client = http.DefaultClient
	}
	var out []byte
	var hdr http.Header
	err := Do(ctx, DefaultConfig, func() error {
		req, err := build()
		if err != nil {
			return err // request construction is deterministic — not transient
		}
		resp, err := client.Do(req)
		if err != nil {
			return &TransientError{Err: err}
		}
		defer resp.Body.Close()
		out, err = httpread.All(resp.Body, maxBytes)
		if err != nil {
			return err
		}
		if resp.StatusCode/100 != 2 {
			return NewHTTPError(resp, string(out))
		}
		hdr = resp.Header // some adapters read usage from response headers (Bedrock M327)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return out, hdr, nil
}

// DoHTTPStream retries the stream SETUP only: it returns the first 2xx
// response with its body still open (the caller consumes the stream and
// closes it). A non-2xx attempt has its body read (bounded) both to classify
// transient-vs-terminal and to free the connection. Failures AFTER a 2xx
// begins are deliberately not retried — a partially delivered stream has
// already surfaced tokens to the caller and must not be replayed.
func DoHTTPStream(ctx context.Context, client *http.Client, build func() (*http.Request, error), maxErrBytes int64) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	var out *http.Response
	err := Do(ctx, DefaultConfig, func() error {
		req, err := build()
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return &TransientError{Err: err}
		}
		if resp.StatusCode/100 != 2 {
			body, _ := httpread.All(resp.Body, maxErrBytes)
			resp.Body.Close()
			return NewHTTPError(resp, string(body))
		}
		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
