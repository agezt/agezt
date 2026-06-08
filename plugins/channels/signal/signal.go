// SPDX-License-Identifier: MIT

// Package signal is an in-process duplex Channel (SPEC-04 §1) for Signal, talking
// to a signal-cli-rest-api server (github.com/bbernhard/signal-cli-rest-api) over
// net/http only — no external dependency. It long-polls GET /v1/receive/{number}
// for inbound messages and POSTs /v2/send for outbound, mirroring how the Matrix
// channel long-polls /sync. signal-cli-rest-api wraps signal-cli behind a small
// HTTP API the operator runs locally (or in their own network), so the URL is
// operator-pinned: there is no SSRF surface, just as with the Home Assistant tool.
//
// Security (SPEC-04 §1.7): inbound is an injection surface. Only senders on the
// allowlist may drive the agent; everyone else is journaled and ignored. The
// account's OWN number is skipped so a reply never re-enters the loop. Inbound
// text is passed to the agent as an intent (data), and the agent's tool calls
// still pass through Edict.
//
// Scope: text messages. Attachments (inbound images → data: URL for vision) are a
// deliberate follow-up, kept out so this first cut stays small and correct.
package signal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

// signalReceiveMaxBytes bounds a /v1/receive JSON response so a buggy or MITM'd
// API server can't stream an unbounded body and OOM the daemon. A drained queue
// of text envelopes is tiny; 8 MiB is far above any legitimate batch. Mirrors
// every other HTTP response cap in the channel set.
const signalReceiveMaxBytes = 8 << 20

// signalMaxChars chunks an outbound message. Signal carries large messages fine,
// but a very long agent answer is split into readable parts rather than sent as
// one wall of text; 2000 chars per part is a comfortable, conservative size.
const signalMaxChars = 2000

// signalMinPollInterval floors the poll rate. signal-cli-rest-api honours
// ?timeout= and blocks for it, so a healthy server already paces us; this only
// kicks in if a server returns empty immediately (older/misconfigured), so we
// never busy-spin and hammer the local API.
const signalMinPollInterval = time.Second

// Config constructs a Channel.
type Config struct {
	APIURL          string // signal-cli-rest-api base URL, e.g. http://127.0.0.1:8080
	Number          string // the registered Signal number this bot is, E.164 (+1…)
	Token           string // optional bearer token (a reverse proxy fronting the API)
	Allowlist       channel.Allowlist
	Bus             *bus.Bus
	Handler         channel.InboundHandler
	HTTPClient      *http.Client
	PollTimeoutSecs int // /v1/receive long-poll seconds; default 10
}

// Channel is the Signal channel.
type Channel struct {
	base     string
	number   string
	token    string
	client   *http.Client
	allow    channel.Allowlist
	bus      *bus.Bus
	handler  channel.InboundHandler
	pollSecs int
}

// New builds a Channel from cfg.
func New(cfg Config) *Channel {
	client := cfg.HTTPClient
	poll := cfg.PollTimeoutSecs
	if poll <= 0 {
		poll = 10
	}
	if client == nil {
		// Timeout must exceed the long-poll window so /v1/receive isn't cut off
		// mid-poll (poll seconds + a margin for the round trip).
		client = &http.Client{Timeout: time.Duration(poll+30) * time.Second}
	}
	return &Channel{
		base:     strings.TrimRight(cfg.APIURL, "/"),
		number:   strings.TrimSpace(cfg.Number),
		token:    cfg.Token,
		client:   client,
		allow:    cfg.Allowlist,
		bus:      cfg.Bus,
		handler:  cfg.Handler,
		pollSecs: poll,
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "signal" }

// --- signal-cli-rest-api wire shapes (only the fields we use) --------------

// envelope is one received message. signal-cli-rest-api returns an array of
// these from GET /v1/receive/{number}; we read the source and the text body.
type envelope struct {
	Envelope struct {
		Source      string `json:"source"`
		Timestamp   int64  `json:"timestamp"`
		DataMessage struct {
			Message   string `json:"message"`
			Timestamp int64  `json:"timestamp"`
		} `json:"dataMessage"`
	} `json:"envelope"`
}

// Start implements channel.Channel: long-poll /v1/receive until ctx is
// cancelled. Per-iteration errors back off briefly and retry; ctx cancellation
// ends the loop cleanly. signal-cli-rest-api drains the queue on each receive, so
// no cursor is needed — every message is returned exactly once.
func (c *Channel) Start(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		start := time.Now()
		msgs, err := c.receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}
		for _, ev := range msgs {
			if !c.dispatchable(ev) {
				continue
			}
			ev := ev
			channel.Guard(c.bus, "signal", func() { c.handleInbound(ctx, ev) })
		}
		// If the server returned an empty batch faster than the floor (i.e. it
		// did not honour ?timeout), pace the next poll so we never busy-spin.
		// When it blocked for the timeout, or there were messages to drain, this
		// is a no-op and we poll again immediately.
		if len(msgs) == 0 {
			if rem := signalMinPollInterval - time.Since(start); rem > 0 {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(rem):
				}
			}
		}
	}
}

// dispatchable reports whether an envelope is an inbound text message from
// someone other than this account itself.
func (c *Channel) dispatchable(ev envelope) bool {
	return strings.TrimSpace(ev.Envelope.DataMessage.Message) != "" &&
		ev.Envelope.Source != "" &&
		ev.Envelope.Source != c.number
}

// receive performs one long-poll GET /v1/receive/{number}.
func (c *Channel) receive(ctx context.Context) ([]envelope, error) {
	q := url.Values{}
	q.Set("timeout", fmt.Sprintf("%d", c.pollSecs))
	path := "/v1/receive/" + url.PathEscape(c.number) + "?" + q.Encode()
	var out []envelope
	if err := c.getJSON(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// handleInbound normalizes one message, enforces the allowlist, runs the
// handler, and replies. All steps journaled so `agt why` can reconstruct it.
func (c *Channel) handleInbound(ctx context.Context, ev envelope) {
	sender := ev.Envelope.Source
	msg := channel.UnifiedMessage{
		ChannelKind:  "signal",
		ChannelID:    sender, // reply goes back to the sender's number
		Sender:       sender,
		Text:         ev.Envelope.DataMessage.Message,
		PlatformTSMS: ev.Envelope.Timestamp,
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(sender)
	c.emitInbound(msg, corr, allowed)

	if !allowed {
		// Fail-closed: a non-allowlisted sender cannot drive the agent. Tell them
		// once so it isn't a silent black hole.
		_ = c.send(ctx, channel.Outbound{ChannelID: sender, Text: "not authorized"}, "")
		return
	}
	if c.handler == nil {
		return
	}
	reply, err := c.handler(ctx, msg, corr)
	if err != nil {
		reply = "sorry — that failed: " + err.Error()
	}
	if reply == "" {
		return
	}
	_ = c.send(ctx, channel.Outbound{ChannelID: sender, Text: reply, Priority: channel.PriorityNotify}, corr)
}

// Send implements channel.Channel (used by the Pulse→Signal sink and any
// out-of-band sender). Journaled under no correlation.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	return c.send(ctx, out, "")
}

// send POSTs /v2/send (chunked to the platform limit) and journals
// channel.outbound under corr.
func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	if strings.TrimSpace(out.Text) == "" {
		return nil // empty/whitespace is a no-op, not a failed send
	}
	for _, chunk := range channel.SplitText(out.Text, signalMaxChars) {
		body, _ := json.Marshal(map[string]any{
			"message":    chunk,
			"number":     c.number,
			"recipients": []string{out.ChannelID},
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v2/send", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		c.authorize(req)
		resp, err := c.client.Do(req)
		if err != nil {
			return c.scrubToken(err)
		}
		err = func() error {
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				return fmt.Errorf("signal send: status %d", resp.StatusCode)
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
			return nil
		}()
		if err != nil {
			return err
		}
	}
	c.emitOutbound(out, corr)
	return nil
}

// getJSON issues a GET and decodes a size-bounded JSON body.
func (c *Channel) getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return c.scrubToken(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, signalReceiveMaxBytes)).Decode(v)
}

// authorize attaches the optional bearer token. signal-cli-rest-api itself is
// unauthenticated; the token is for an operator's fronting reverse proxy.
func (c *Channel) authorize(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

// scrubToken removes the bearer token from an error message — defense in depth in
// case a transport error ever embeds it.
func (c *Channel) scrubToken(err error) error {
	if err == nil || c.token == "" {
		return err
	}
	if msg := err.Error(); strings.Contains(msg, c.token) {
		return errors.New(strings.ReplaceAll(msg, c.token, "<redacted>"))
	}
	return err
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.signal",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-signal",
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
		Subject:       "channel.outbound.signal",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-signal",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "signal",
			"channel_id":   out.ChannelID,
			"text":         out.Text,
			"priority":     string(out.Priority),
		},
	})
}
