// SPDX-License-Identifier: MIT

// Package telegram is an in-process duplex Channel (SPEC-04 §1) over the
// Telegram Bot API, using net/http only — no external dependency. It
// long-polls getUpdates for inbound messages and POSTs sendMessage for
// outbound. The same Channel interface an out-of-process plugin will satisfy
// later (SPEC-04 §1.6); in-process is the Phase-4 MVP choice, matching how
// memory/pulse run inside the daemon.
//
// Security (SPEC-04 §1.7): inbound is an injection surface. Only chat ids on
// the allowlist may drive the agent; everyone else is journaled and ignored.
// Inbound text is passed to the agent as an intent (data), and the agent's
// tool calls still pass through Edict.
package telegram

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
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
}

// tgPhotoSize is one rendition of an inbound photo. Telegram sends several
// sizes; the agent wants the largest for the clearest vision input.
type tgPhotoSize struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
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
	return m != nil && (m.Text != "" || m.Caption != "" || len(m.Photo) > 0)
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

func (c *Channel) getUpdates(ctx context.Context) ([]tgUpdate, error) {
	q := url.Values{}
	q.Set("timeout", strconv.Itoa(c.pollSecs))
	if c.offset > 0 {
		q.Set("offset", strconv.FormatInt(c.offset, 10))
	}
	endpoint := fmt.Sprintf("%s/bot%s/getUpdates?%s", c.base, c.token, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, c.scrubToken(err)
	}
	defer resp.Body.Close()
	var out getUpdatesResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, tgAPIMaxResponseBytes)).Decode(&out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("telegram getUpdates: not ok")
	}
	return out.Result, nil
}

// handleInbound normalizes one message, enforces the allowlist, runs the
// handler, and replies. All steps journaled so `agt why`/`agt inbox` can
// reconstruct the exchange.
func (c *Channel) handleInbound(ctx context.Context, m *tgMessage) {
	chatID := strconv.FormatInt(m.Chat.ID, 10)
	sender := chatID
	if m.From != nil && m.From.Username != "" {
		sender = m.From.Username
	}
	// A photo carries its text as a caption, not in Text.
	text := m.Text
	if text == "" {
		text = m.Caption
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "telegram",
		ChannelID:    chatID,
		Sender:       sender,
		Text:         text,
		PlatformTSMS: m.Date * 1000,
		PlatformMeta: map[string]string{"message_id": strconv.FormatInt(m.MessageID, 10)},
	}

	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(chatID)
	// Inbound photo (M247): fetch the largest size as a data: URL so a vision
	// model can see it. Only for allowlisted senders — never dereference a
	// file reference from an unauthorized sender.
	if allowed && len(m.Photo) > 0 {
		largest := m.Photo[len(m.Photo)-1]
		if du, err := c.fetchPhotoDataURL(ctx, largest.FileID); err == nil && du != "" {
			msg.Images = []string{du}
		}
	}
	c.emitInbound(msg, corr, allowed)

	if !allowed {
		// Fail-closed: a non-allowlisted sender cannot drive the agent.
		// Tell them once so it isn't a silent black hole.
		_ = c.send(ctx, channel.Outbound{ChannelID: chatID, Text: "not authorized"}, "")
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
	_ = c.send(ctx, channel.Outbound{ChannelID: chatID, Text: reply, Priority: channel.PriorityNotify}, corr)
}

// tgPhotoMaxRaw bounds a downloaded photo so the resulting data: URL stays
// within the control-plane request cap (16 MiB; base64 ≈ 4/3 × raw).
const tgPhotoMaxRaw = 12 << 20

// fetchPhotoDataURL resolves a Telegram photo file_id to an inline data: URL.
// Telegram needs two calls: getFile to learn the file_path, then a download
// from the /file/bot<token>/ endpoint. The bytes are read here in the channel
// (the daemon holds the bot token, not the provider) and handed onward as a
// self-describing data: URL the vision providers emit natively (M247).
func (c *Channel) fetchPhotoDataURL(ctx context.Context, fileID string) (string, error) {
	gf := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", c.base, c.token, url.QueryEscape(fileID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gf, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", c.scrubToken(err)
	}
	var gfResp struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := func() error {
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("telegram getFile: status %d", resp.StatusCode)
		}
		return json.NewDecoder(io.LimitReader(resp.Body, tgAPIMaxResponseBytes)).Decode(&gfResp)
	}(); err != nil {
		return "", err
	}
	if !gfResp.OK || gfResp.Result.FilePath == "" {
		return "", fmt.Errorf("telegram getFile: no file_path")
	}

	dl := fmt.Sprintf("%s/file/bot%s/%s", c.base, c.token, gfResp.Result.FilePath)
	dreq, err := http.NewRequestWithContext(ctx, http.MethodGet, dl, nil)
	if err != nil {
		return "", err
	}
	dresp, err := c.client.Do(dreq)
	if err != nil {
		return "", c.scrubToken(err)
	}
	defer dresp.Body.Close()
	if dresp.StatusCode/100 != 2 {
		return "", fmt.Errorf("telegram file download: status %d", dresp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(dresp.Body, tgPhotoMaxRaw+1))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("telegram file download: empty")
	}
	if len(data) > tgPhotoMaxRaw {
		return "", fmt.Errorf("telegram photo exceeds %d bytes", tgPhotoMaxRaw)
	}
	return "data:" + tgMediaType(gfResp.Result.FilePath) + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// tgMediaType maps a Telegram file_path extension to an image media type.
// Telegram photos are JPEG; stickers/other uploads may differ.
func tgMediaType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

// Send implements channel.Channel (used by the Pulse→Telegram sink and any
// out-of-band sender). Journaled under no correlation.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	return c.send(ctx, out, "")
}

// telegramMaxChars is Telegram's per-message limit (4096 UTF-16 code units).
// A longer sendMessage is rejected with 400, so a long answer is split into
// sequential messages rather than lost (M234).
const telegramMaxChars = 4096

// send POSTs sendMessage (chunked to the platform limit) and journals
// channel.outbound under corr.
func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	// An empty or whitespace-only message is rejected by Telegram (400
	// "message text is empty"). Treat it as a no-op rather than a failed send —
	// covers the Send path (Pulse, agt send) and whitespace-only agent answers
	// the inbound reply guard's exact-"" check would miss (M236).
	if strings.TrimSpace(out.Text) == "" {
		return nil
	}
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", c.base, c.token)
	for _, chunk := range channel.SplitText(out.Text, telegramMaxChars) {
		body, _ := json.Marshal(map[string]any{"chat_id": out.ChannelID, "text": chunk})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.client.Do(req)
		if err != nil {
			return c.scrubToken(err)
		}
		// Drain+close before the next iteration so the connection is reused.
		err = func() error {
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				return fmt.Errorf("telegram sendMessage: status %d", resp.StatusCode)
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	c.emitOutbound(out, corr)
	return nil
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.telegram",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-telegram",
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
		Subject:       "channel.outbound.telegram",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-telegram",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "telegram",
			"channel_id":   out.ChannelID,
			"text":         out.Text,
			"priority":     string(out.Priority),
		},
	})
}
