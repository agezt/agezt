// SPDX-License-Identifier: MIT

package webhook

// Webhook helpers + parsing + probe: sign + verb + loopbackHost +
// ParseSinks + ProbeResult + Probe + Describe. Carved out of
// webhook.go during the Day 190 god-file split so the main file
// can stay focused on types + lifecycle and the dispatch file can
// stay focused on the delivery mechanics.
// Public API unchanged.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func verb(kind event.Kind) string {
	s := string(kind)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// ParseSinks parses the AGEZT_WEBHOOKS spec: a comma-separated list of sinks,
// each "url|subject|secret" (subject and secret optional; subject defaults to
// ">"). Whitespace around fields is trimmed. URLs must be http(s) and contain no
// comma. A malformed entry is a hard error so a misconfigured webhook is caught
// at startup, not silently dropped.

// loopbackHost reports whether host is a loopback address (localhost name or
// 127.0.0.0/8 / ::1 IP literal). Used by ParseSinks to permit http:// URLs
// only for local development (no TLS cert needed on the dev machine).
func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Hostname — not a loopback literal.
		// (A hostname that resolves to 127.x.x.x is a separate concern;
		// the dial-level netguard blocks it unless AllowLoopback is set.)
		return false
	}
	return ip.IsLoopback()
}

func ParseSinks(spec string) ([]Sink, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var sinks []Sink
	for _, raw := range strings.Split(spec, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		// Bounded split (M218): the secret is the LAST field and an HMAC key may
		// legitimately contain '|'. An unbounded Split would put only the text up to the
		// third '|' into the secret and silently drop the rest, corrupting the key so
		// every signature mismatches at the receiver — a silent delivery failure. SplitN
		// with 3 keeps everything after the second '|' as the secret.
		parts := strings.SplitN(entry, "|", 3)
		s := Sink{Subject: ">"}
		s.URL = strings.TrimSpace(parts[0])
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			s.Subject = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			s.Secret = strings.TrimSpace(parts[2])
		}
		u, err := url.Parse(s.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("webhook: invalid URL %q (need http(s)://host…)", s.URL)
		}
		// Non-loopback sinks MUST use HTTPS so journal payloads are not
		// exfiltrated in plaintext over the network. Loopback is exempt
		// so local development (e.g. a local webhook listener) still
		// works without a TLS cert.
		if u.Scheme == "http" && !loopbackHost(u.Hostname()) {
			return nil, fmt.Errorf("webhook: non-loopback sink %q must use https:// (http:// allowed only for loopback)", s.URL)
		}
		// A malformed subject filter never matches any event, so the sink would
		// silently deliver nothing. Reject it at parse time instead (M217).
		if err := bus.ValidatePattern(s.Subject); err != nil {
			return nil, fmt.Errorf("webhook: invalid subject filter %q: %w", s.Subject, err)
		}
		sinks = append(sinks, s)
	}
	return sinks, nil
}

// TestEventKind is the Kind carried by a probe delivery (agt webhook test). A
// receiver sees it in the X-Agezt-Event header and can recognize a ping rather
// than acting on it as a real event.
const TestEventKind = "webhook.test"

// testEventID is the stable, obviously-synthetic delivery id a probe sends, so a
// receiver that dedupes on X-Agezt-Delivery can tell test pings apart.
const testEventID = "00000000000000000000000000"

// ProbeResult reports the outcome of a single test delivery.
type ProbeResult struct {
	URL     string        `json:"url"`
	Subject string        `json:"subject"`
	Signed  bool          `json:"signed"`
	Status  int           `json:"status"`
	Latency time.Duration `json:"latency_ns"`
	Err     string        `json:"error,omitempty"`
}

// OK reports whether the probe received a 2xx response.
func (r ProbeResult) OK() bool { return r.Err == "" && r.Status >= 200 && r.Status < 300 }

// Probe sends a single synthetic webhook.test event to sink and reports the
// outcome, using the byte-identical body, headers, and HMAC signature the live
// dispatcher uses — so a 2xx here means real deliveries will be accepted too. It
// is the daemon-free verification behind `agt webhook test`: an operator who just
// configured a sink can confirm it is reachable, accepts the format, and (if
// signed) validates the signature, without waiting for a real event to fire.
// Unlike a real delivery it does NOT retry — a test wants the immediate truth,
// not a transient masked by backoff. now stamps the synthetic event; client may
// be nil (a DefaultTimeout client is used).
func Probe(ctx context.Context, sink Sink, now time.Time, client *http.Client) ProbeResult {
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	subject := sink.Subject
	if subject == "" {
		subject = ">"
	}
	res := ProbeResult{URL: sink.URL, Subject: subject, Signed: sink.Secret != ""}
	ev := &event.Event{
		ID:       testEventID,
		TSUnixMS: now.UnixMilli(),
		Subject:  subject,
		Actor:    "agezt",
		Kind:     TestEventKind,
		Payload:  json.RawMessage(`{"test":true,"message":"agezt webhook test probe"}`),
	}
	body, err := json.Marshal(ev)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	req, err := newDeliveryRequest(ctx, sink, body, ev, ev.ID)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	start := time.Now()
	resp, err := client.Do(req)
	res.Latency = time.Since(start)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	res.Status = resp.StatusCode
	return res
}

// Describe renders a one-line banner summary of the sinks (secrets redacted).
func Describe(sinks []Sink) string {
	if len(sinks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(sinks))
	for _, s := range sinks {
		sig := ""
		if s.Secret != "" {
			sig = " (signed)"
		}
		parts = append(parts, fmt.Sprintf("%s → %s%s", s.Subject, s.URL, sig))
	}
	return fmt.Sprintf("%d sink(s): %s", len(sinks), strings.Join(parts, ", "))
}

