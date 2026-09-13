// SPDX-License-Identifier: MIT

// Telegram channel: types + lifecycle + receive flow + Send dispatcher + emit helpers.
// Code extracted from telegram.go during the Day-95 god-file split.
// Public API unchanged.
package telegram

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
)


// DefaultBaseURL is the public Bot API root.
const DefaultBaseURL = "https://api.telegram.org"

// tgAPIMaxResponseBytes bounds a Telegram Bot API JSON response (getUpdates /
// getFile) so a buggy, compromised, or MITM'd endpoint can't stream an unbounded
// body and OOM the daemon. 8 MiB is far above any legitimate getUpdates batch.
// Mirrors the size cap every other HTTP response in the tree already carries; the
// photo download has its own (tgPhotoMaxRaw).
const tgAPIMaxResponseBytes = 8 << 20

// Config constructs a Channel.
type Config struct {
	Token           string
	BaseURL         string // default DefaultBaseURL; override for tests
	HTTPClient      *http.Client
	Allowlist       channel.Allowlist
	Bus             *bus.Bus
	Handler         channel.InboundHandler
	PollTimeoutSecs int // long-poll seconds; default 25
}

// Channel is the Telegram channel.
type Channel struct {
	token    string
	base     string
	client   *http.Client
	allow    channel.Allowlist
	bus      *bus.Bus
	handler  channel.InboundHandler
	pollSecs int

	offset int64 // getUpdates offset (last processed update_id + 1)
}

// New builds a Channel from cfg.
func New(cfg Config) *Channel {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	client := cfg.HTTPClient
	if client == nil {
		// Timeout must exceed the long-poll window so getUpdates isn't
		// cut off mid-poll.
		client = &http.Client{Timeout: 60 * time.Second}
	}
	poll := cfg.PollTimeoutSecs
	if poll <= 0 {
		poll = 25
	}
	return &Channel{
		token:    cfg.Token,
		base:     base,
		client:   client,
		allow:    cfg.Allowlist,
		bus:      cfg.Bus,
		handler:  cfg.Handler,
		pollSecs: poll,
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "telegram" }

// --- Bot API wire shapes (only the fields we use) -------------------------

type tgUpdate struct {
	UpdateID int64      `json:"update_id"`
	Message  *tgMessage `json:"message"`
}

type tgMessage struct {
	MessageID int64         `json:"message_id"`
	From      *tgUser       `json:"from"`
	Chat      tgChat        `json:"chat"`
	Date      int64         `json:"date"`
	Text      string        `json:"text"`
	Caption   string        `json:"caption"` // a photo's text rides here, not in Text
	Photo     []tgPhotoSize `json:"photo"`   // ascending sizes; the last is largest
	Voice     *tgVoice      `json:"voice"`   // a voice note (OGG/Opus) — auto-transcribed downstream
	// Forum-topic threading (M885): in a forum supergroup each topic carries
	// message_thread_id + is_topic_message. The flag matters — a plain REPLY in
	// a non-forum chat also sets message_thread_id, and treating that as a
	// conversation boundary would split ordinary chats.
	MessageThreadID int64 `json:"message_thread_id"`
	IsTopicMessage  bool  `json:"is_topic_message"`
}

// tgPhotoSize is one rendition of an inbound photo. Telegram sends several
// sizes; the agent wants the largest for the clearest vision input.
type tgPhotoSize struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int    `json:"file_size"`
}

// tgVoice is an inbound Telegram voice note (OGG/Opus). Only the file_id is
// needed to resolve the bytes via getFile (mirrors a photo).
type tgVoice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
	MimeType string `json:"mime_type"`
	FileSize int    `json:"file_size"`
}

type tgUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

type getUpdatesResp struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

// Start implements channel.Channel: long-poll getUpdates until ctx is
// cancelled. Per-iteration errors back off briefly and retry (a flaky network
// shouldn't kill the channel); ctx cancellation ends the loop cleanly.
func (c *Channel) Start(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		updates, err := c.getUpdates(ctx)
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
		for _, u := range updates {
			c.offset = u.UpdateID + 1
			if !dispatchable(u.Message) {
				continue
			}
			channel.Guard(c.bus, "telegram", func() { c.handleInbound(ctx, u.Message) })
		}
	}
}

// dispatchable reports whether an inbound update carries content worth handling.
// A photo rides its text in Caption (not Text) and may have no text at all, so
// gating only on Text != "" silently dropped photo/caption-only messages before
// they reached handleInbound — killing the inbound-image path (M247) on the live
// poll loop, even though handleInbound fully supports it. (M476)
func dispatchable(m *tgMessage) bool {
	return m != nil && (m.Text != "" || m.Caption != "" || len(m.Photo) > 0 || m.Voice != nil)
}

// scrubToken removes the bot token from an error message. http.Client.Do returns
// a *url.Error whose text embeds the full request URL, and the Telegram API puts
// the token in the URL path (/bot<token>/…) — so a transport failure (DNS,
// refused, timeout) would otherwise carry the secret into any log/journal that
// records the error. Applied at every Do() error return.
func (c *Channel) scrubToken(err error) error {
	if err == nil || c.token == "" {
		return err
	}
	if msg := err.Error(); strings.Contains(msg, c.token) {
		return errors.New(strings.ReplaceAll(msg, c.token, "<redacted>"))
	}
	return err
}

