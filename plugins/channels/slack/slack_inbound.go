// SPDX-License-Identifier: MIT

package slack

// Slack inbound receive flow: slackEnvelope + slackEvent +
// slackFile types + handleEvents + process. Carved out of
// slack.go during the Day 200 god-file split so the main file
// can stay focused on types + lifecycle + verify and the
// existing slack_send.go can stay focused on outbound Send +
// shared emit/parse helpers.
// Public API unchanged.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/ulid"
)

type slackEnvelope struct {
	Type      string      `json:"type"`      // url_verification | event_callback
	Challenge string      `json:"challenge"` // url_verification handshake
	Event     *slackEvent `json:"event"`
}

type slackEvent struct {
	Type     string      `json:"type"`    // "message"
	Channel  string      `json:"channel"` // C…
	User     string      `json:"user"`    // U…
	Text     string      `json:"text"`
	TS       string      `json:"ts"`
	ThreadTS string      `json:"thread_ts"` // set when the message lives in a thread (M885)
	BotID    string      `json:"bot_id"`    // set when the message is from a bot
	Subtype  string      `json:"subtype"`   // set for edits/joins/bot_message/etc.
	Files    []slackFile `json:"files"`     // shared-file attachments
}

// slackFile is an inbound file attachment. url_private requires the bot token in
// an Authorization header to download; mimetype tells us whether it's an image.
type slackFile struct {
	URLPrivate string `json:"url_private"`
	Mimetype   string `json:"mimetype"`
	Name       string `json:"name"`
}

func (c *Channel) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if !c.verify(r.Header.Get("X-Slack-Request-Timestamp"), r.Header.Get("X-Slack-Signature"), body) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	var env slackEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	// URL-verification handshake: echo the challenge so Slack accepts the
	// endpoint. (Sent once when the operator configures the Events URL.)
	if env.Type == "url_verification" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": env.Challenge})
		return
	}

	// Everything else: ACK immediately (Slack needs 200 within 3s and retries
	// otherwise) and process asynchronously. A retry delivery (X-Slack-Retry-Num)
	// is ACKed but not reprocessed, so a slow agent run can't be double-handled.
	w.WriteHeader(http.StatusOK)
	if r.Header.Get("X-Slack-Retry-Num") != "" {
		return
	}
	if env.Type != "event_callback" || env.Event == nil {
		return
	}
	ev := *env.Event
	// Only real user messages drive the agent. Ignore bot/self messages and
	// message subtypes (edits, joins, channel_topic, bot_message) — replying to
	// our own posts would loop.
	if ev.Type != "message" || ev.BotID != "" || ev.Subtype != "" || ev.User == "" || ev.Text == "" {
		return
	}
	// Replay guard: the signature's freshness window still permits replay of a
	// captured signed body (without the retry header) within 5 minutes. Key on the
	// immutable channel+ts so each message drives at most one run.
	if c.dedup.seenBefore(ev.Channel + ":" + ev.TS) {
		return
	}
	// Detach from the request context (which ends when we return the ACK); the
	// async run uses a background context so it survives the HTTP response.
	go channel.Guard(c.bus, "slack", func() { c.process(c.baseCtx, ev) })
}

// process normalizes one message, enforces the allowlist, runs the handler, and
// posts the reply. Journaled so `agt why`/`agt inbox` can reconstruct it.
func (c *Channel) process(ctx context.Context, ev slackEvent) {
	msg := channel.UnifiedMessage{
		ChannelKind:  "slack",
		ChannelID:    ev.Channel,
		ThreadID:     ev.ThreadTS, // M885: a Slack thread is its own conversation
		Sender:       ev.User,
		Text:         ev.Text,
		PlatformTSMS: slackTSMillis(ev.TS),
		PlatformMeta: map[string]string{"ts": ev.TS},
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(ev.Channel)
	// Inbound image files (M248): fetch each shared image as a data: URL so a
	// vision model can see it. Only for allowlisted senders — never download a
	// file referenced by an unauthorized sender.
	if allowed && len(ev.Files) > 0 {
		for _, f := range ev.Files {
			if f.URLPrivate == "" {
				continue
			}
			switch {
			case strings.HasPrefix(f.Mimetype, "image/"):
				if du, err := c.fetchFileDataURL(ctx, f.URLPrivate, f.Mimetype); err == nil && du != "" {
					msg.Images = append(msg.Images, du)
				}
			case strings.HasPrefix(f.Mimetype, "audio/"):
				// Voice clips / audio files → transcribed by the ambient STT path.
				if du, err := c.fetchFileDataURL(ctx, f.URLPrivate, f.Mimetype); err == nil && du != "" {
					msg.Audio = append(msg.Audio, du)
				}
			}
		}
	}
	c.emitInbound(msg, corr, allowed)

	if !allowed {
		_ = c.send(ctx, channel.Outbound{ChannelID: ev.Channel, Text: "not authorized"}, "")
		return
	}
	if c.handler == nil {
		return
	}
	rep, err := c.handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	reply := rep.Text
	if reply == "" && len(rep.Attachments) == 0 {
		return
	}
	_ = c.send(ctx, channel.Outbound{ChannelID: ev.Channel, ThreadID: msg.ThreadID, Text: reply, Attachments: rep.Attachments, Priority: channel.PriorityNotify}, corr)
}

// verify checks Slack's request signature: v0=HMAC-SHA256(secret, "v0:ts:body"),
// with a timestamp freshness window for replay protection. An empty secret fails
// closed (no inbound without a configured signing secret).
