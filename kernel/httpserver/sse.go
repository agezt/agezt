// SPDX-License-Identifier: MIT

package httpserver

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/agezt/agezt/kernel/streamlimit"
)

// maxSSEPerClient bounds concurrent Server-Sent-Events streams from one client
// IP across EVERY HTTP surface of the daemon (V-009). Long-lived streams
// (events firehose, mailbox watch, chat token relays, install progress) are
// otherwise uncapped, so a leaked token or a buggy client could exhaust file
// descriptors and goroutines by opening streams in a loop. Generous enough to
// never trip for legitimate use — it fences the pathological case only.
//
// One process-wide limiter: before this existed, webui and restapi each had a
// private copy of the gate and the other seven SSE endpoints had none (LD-8),
// so the cap was one implementation wide out of nine.
const maxSSEPerClient = 64

var sseLimiter = streamlimit.New(maxSSEPerClient)

// AcquireSSE reserves an SSE slot for the request's client IP. On success it
// returns a release func (defer it) and true. Over the cap it writes 429 +
// Retry-After and returns false — the handler must return without streaming.
func AcquireSSE(w http.ResponseWriter, r *http.Request) (release func(), ok bool) {
	release, ok = sseLimiter.Acquire(sseClientKey(r))
	if !ok {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "too many concurrent streams from this client", http.StatusTooManyRequests)
	}
	return release, ok
}

// sseClientKey identifies the client for stream-cap accounting: the host part
// of RemoteAddr. Behind a reverse proxy all clients share the proxy's address,
// so the cap is per-proxy there — acceptable for a loopback-default daemon;
// the cap bounds abuse, it doesn't meter tenants.
func sseClientKey(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// SSEStream is an open Server-Sent-Events response: headers sent, per-client
// slot held, flusher resolved. Writes are best-effort (a broken client surfaces
// on the next write's error return from the underlying writer; SSE handlers
// conventionally just stop on their context instead).
type SSEStream struct {
	w       http.ResponseWriter
	fl      http.Flusher
	release func()
}

// StartSSE opens an SSE response: verifies the writer can stream (500 when
// not), reserves the per-client slot (429 when over the cap), writes the SSE
// header triple, and flushes a 200. On ok=false the error response has already
// been written. Callers must defer stream.Close().
func StartSSE(w http.ResponseWriter, r *http.Request) (*SSEStream, bool) {
	fl, canFlush := w.(http.Flusher)
	if !canFlush {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return nil, false
	}
	release, ok := AcquireSSE(w, r)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	return &SSEStream{w: w, fl: fl, release: release}, true
}

// Close releases the per-client stream slot. Idempotent.
func (s *SSEStream) Close() {
	if s.release != nil {
		s.release()
		s.release = nil
	}
}

// WriteJSON emits `data: <json>\n\n` and flushes.
func (s *SSEStream) WriteJSON(obj any) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return s.WriteData(string(b))
}

// WriteEvent emits `event: <name>\ndata: <json>\n\n` and flushes.
func (s *SSEStream) WriteEvent(name string, obj any) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = s.w.Write([]byte("event: " + name + "\ndata: " + string(b) + "\n\n"))
	s.fl.Flush()
	return err
}

// WriteData emits `data: <raw>\n\n` (raw is already-encoded payload or a
// sentinel like "[DONE]") and flushes.
func (s *SSEStream) WriteData(raw string) error {
	_, err := s.w.Write([]byte("data: " + raw + "\n\n"))
	s.fl.Flush()
	return err
}

// Comment emits an SSE comment line (`: <text>\n\n`) and flushes — used as an
// open-of-stream signal (EventSource fires onopen) and as a keep-alive ping.
func (s *SSEStream) Comment(text string) error {
	_, err := s.w.Write([]byte(": " + text + "\n\n"))
	s.fl.Flush()
	return err
}
