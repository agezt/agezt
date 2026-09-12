// SPDX-License-Identifier: MIT

// SMS channel: twilioSignature + dedup (pure crypto/cache helpers).
// Code extracted from sms.go during the Day-134 god-file split.
// Public API unchanged.
package sms


import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

func twilioSignature(token, fullURL string, form url.Values) string {
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(fullURL)
	for _, k := range keys {
		b.WriteString(k)
		// Multi-valued params concatenate their values in order.
		for _, v := range form[k] {
			b.WriteString(v)
		}
	}
	mac := hmac.New(sha1.New, []byte(token))
	mac.Write([]byte(b.String()))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// --- outbound -------------------------------------------------------------

// Send implements channel.Channel: send an SMS to out.ChannelID (a phone number)
// via the Twilio REST API, splitting long text. Errors when no From number or
// credentials are configured.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	if strings.TrimSpace(out.Text) == "" {
		return nil
	}
	if c.from == "" || c.sid == "" || c.token == "" {
		return fmt.Errorf("sms: outbound not configured (set AccountSID, AuthToken, From)")
	}
	corr := "chan-" + ulid.New()
	for _, chunk := range channel.SplitText(out.Text, smsMaxChars) {
		if err := c.sendOne(ctx, out.ChannelID, chunk); err != nil {
			return err
		}
	}
	c.emitOutbound(out, corr)
	return nil
}

func (c *Channel) sendOne(ctx context.Context, to, body string) error {
	endpoint := c.apiBase + "/2010-04-01/Accounts/" + url.PathEscape(c.sid) + "/Messages.json"
	form := url.Values{}
	form.Set("To", to)
	form.Set("From", c.from)
	form.Set("Body", body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.sid, c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("sms: Twilio API returned status %d", resp.StatusCode)
	}
	return nil
}

// --- events ---------------------------------------------------------------

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.sms",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-sms",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": msg.ChannelKind,
			"channel_id":   msg.ChannelID,
			"sender":       msg.Sender,
			"text":         msg.Text,
			"allowed":      allowed,
		},
	})
}

func (c *Channel) emitOutbound(out channel.Outbound, corr string) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.outbound.sms",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-sms",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_id": out.ChannelID,
			"text":       out.Text,
			"priority":   string(out.Priority),
		},
	})
}

// --- helpers --------------------------------------------------------------

// writeTwiML writes a TwiML response. An empty reply yields a bare <Response/>
// (acknowledge, no message). The reply text is XML-escaped.
func writeTwiML(w http.ResponseWriter, reply string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if reply == "" {
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Response></Response>`)
		return
	}
	_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Response><Message>`+xmlEscape(reply)+`</Message></Response>`)
}

// xmlEscape escapes the five XML predefined entities so a reply can't break the
// TwiML envelope.
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// dedup is a small bounded set of recently-seen MessageSids (replay/retry guard).
// It keeps two generations so eviction never forgets every id at once: a key is
// dropped only after it ages out of both, bounding memory at 2×cap. Mirrors the
// webhook channel's guard.
type dedup struct {
	mu   sync.Mutex
	seen map[string]struct{}
	prev map[string]struct{}
	cap  int
}

func newDedup(capacity int) *dedup {
	return &dedup{seen: make(map[string]struct{}, capacity), cap: capacity}
}

func (d *dedup) seenBefore(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[key]; ok {
		return true
	}
	if _, ok := d.prev[key]; ok {
		return true
	}
	if len(d.seen) >= d.cap {
		d.prev = d.seen
		d.seen = make(map[string]struct{}, d.cap)
	}
	d.seen[key] = struct{}{}
	return false
}
