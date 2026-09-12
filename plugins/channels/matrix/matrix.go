// SPDX-License-Identifier: MIT

// Matrix channel: types + lifecycle + receive + send + emit.
// Code extracted from matrix.go during the Day-111 god-file split.
// Public API unchanged.
package matrix


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
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/ulid"
)


// matrixSyncMaxBytes bounds a /sync JSON response so a buggy, compromised, or
// MITM'd homeserver can't stream an unbounded body and OOM the daemon. The sync
// is filtered to a small per-room timeline (see syncFilter), so 8 MiB is far
// above any legitimate incremental batch. Mirrors every other HTTP response cap.
const matrixSyncMaxBytes = 8 << 20

// matrixMaxChars chunks an outbound message. Matrix caps a single event's total
// size at ~64 KiB on most homeservers; 32 KiB of body leaves ample headroom for
// the JSON envelope, and a longer answer is split rather than rejected.
const matrixMaxChars = 32 << 10

// syncFilter bounds each /sync: only m.room.message timeline events, capped per
// room, and no presence/account spam. URL-encoded into the sync request so even
// the initial full sync stays small.
const syncFilter = `{"room":{"timeline":{"limit":20,"types":["m.room.message"]},` +
	`"ephemeral":{"types":[]},"account_data":{"types":[]}},` +
	`"presence":{"types":[]},"account_data":{"types":[]}}`

// Config constructs a Channel.
type Config struct {
	Homeserver      string // base URL, e.g. https://matrix.org (no trailing /_matrix)
	Token           string // access token (operator-provisioned bot account)
	Allowlist       channel.Allowlist
	Bus             *bus.Bus
	Handler         channel.InboundHandler
	HTTPClient      *http.Client
	PollTimeoutSecs int // /sync long-poll seconds; default 30
}

// Channel is the Matrix channel.
type Channel struct {
	base     string
	token    string
	client   *http.Client
	allow    channel.Allowlist
	bus      *bus.Bus
	handler  channel.InboundHandler
	pollSecs int

	userID string // the bot's own MXID, resolved at Start; own events are skipped
	since  string // /sync next_batch cursor
}

// New builds a Channel from cfg.
func New(cfg Config) *Channel {
	client := cfg.HTTPClient
	if client == nil {
		// Timeout must exceed the long-poll window so /sync isn't cut off mid-poll.
		client = &http.Client{Timeout: 60 * time.Second}
	}
	poll := cfg.PollTimeoutSecs
	if poll <= 0 {
		poll = 30
	}
	return &Channel{
		base:     strings.TrimRight(cfg.Homeserver, "/"),
		token:    cfg.Token,
		client:   client,
		allow:    cfg.Allowlist,
		bus:      cfg.Bus,
		handler:  cfg.Handler,
		pollSecs: poll,
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "matrix" }

// --- Client-Server API wire shapes (only the fields we use) ----------------

type whoamiResp struct {
	UserID string `json:"user_id"`
}

type syncResp struct {
	NextBatch string `json:"next_batch"`
	Rooms     struct {
		Join map[string]struct {
			Timeline struct {
				Events []roomEvent `json:"events"`
			} `json:"timeline"`
		} `json:"join"`
	} `json:"rooms"`
}

type roomEvent struct {
	Type    string `json:"type"`
	Sender  string `json:"sender"`
	EventID string `json:"event_id"`
	TS      int64  `json:"origin_server_ts"`
	Content struct {
		MsgType string `json:"msgtype"`
		Body    string `json:"body"`
		URL     string `json:"url"` // mxc:// URI for m.image/m.audio/m.video/m.file
		Info    struct {
			MimeType string `json:"mimetype"`
		} `json:"info"`
	} `json:"content"`
}

// Start implements channel.Channel: resolve the bot's own MXID, prime the sync
// cursor (skipping backlog so a restart doesn't replay old messages), then
// long-poll /sync until ctx is cancelled. Per-iteration errors back off briefly
// and retry; ctx cancellation ends the loop cleanly.
func (c *Channel) Start(ctx context.Context) error {
	// Resolve own MXID so the bot never processes (and replies to) its own
	// messages — without this an allowlisted room would loop forever.
	if err := c.resolveWhoami(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("matrix: whoami: %w", err)
	}
	// Prime the cursor: an initial /sync establishes next_batch; its timeline is
	// the room backlog, which we skip — the channel handles messages from now on.
	if err := c.prime(ctx); err != nil && ctx.Err() == nil {
		// Non-fatal: fall through to the poll loop, which retries with backoff.
		c.logBackoff()
	}

	for {
		if ctx.Err() != nil {
			return nil
		}
		batch, err := c.sync(ctx)
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
		for roomID, room := range batch.Rooms.Join {
			for _, ev := range room.Timeline.Events {
				if !c.dispatchable(ev) {
					continue
				}
				ev, roomID := ev, roomID
				channel.Guard(c.bus, "matrix", func() { c.handleInbound(ctx, roomID, ev) })
			}
		}
		c.since = batch.NextBatch
	}
}

// dispatchable reports whether a timeline event is an inbound message we handle
// (text, or an image/voice with a downloadable mxc:// URI) from someone other
// than the bot itself.
func (c *Channel) dispatchable(ev roomEvent) bool {
	if ev.Type != "m.room.message" || ev.Sender == c.userID {
		return false
	}
	switch ev.Content.MsgType {
	case "m.text":
		return strings.TrimSpace(ev.Content.Body) != ""
	case "m.image", "m.audio", "m.video":
		return strings.HasPrefix(ev.Content.URL, "mxc://")
	}
	return false
}

func (c *Channel) logBackoff() {} // hook point; kept tiny so Start reads cleanly

// resolveWhoami fetches the bot's own MXID.
func (c *Channel) resolveWhoami(ctx context.Context) error {
	var out whoamiResp
	if err := c.getJSON(ctx, "/_matrix/client/v3/account/whoami", &out); err != nil {
		return err
	}
	if out.UserID == "" {
		return errors.New("empty user_id")
	}
	c.userID = out.UserID
	return nil
}

// prime does one /sync to capture next_batch without processing the backlog.
func (c *Channel) prime(ctx context.Context) error {
	q := url.Values{}
	q.Set("timeout", "0")
	q.Set("filter", syncFilter)
	var out syncResp
	if err := c.getJSON(ctx, "/_matrix/client/v3/sync?"+q.Encode(), &out); err != nil {
		return err
	}
	c.since = out.NextBatch
	return nil
}

// sync performs one long-poll /sync from the current cursor.
func (c *Channel) sync(ctx context.Context) (*syncResp, error) {
	q := url.Values{}
	q.Set("timeout", fmt.Sprintf("%d", c.pollSecs*1000))
	q.Set("filter", syncFilter)
	if c.since != "" {
		q.Set("since", c.since)
	}
	var out syncResp
	if err := c.getJSON(ctx, "/_matrix/client/v3/sync?"+q.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// handleInbound normalizes one message, enforces the allowlist, runs the
// handler, and replies. All steps journaled so `agt why` can reconstruct it.
func (c *Channel) handleInbound(ctx context.Context, roomID string, ev roomEvent) {
	msg := channel.UnifiedMessage{
		ChannelKind:  "matrix",
		ChannelID:    roomID,
		Sender:       ev.Sender,
		Text:         ev.Content.Body,
		PlatformTSMS: ev.TS,
		PlatformMeta: map[string]string{"event_id": ev.EventID},
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(roomID)
	// Inbound media: download the mxc:// content (allowlisted rooms only — never
	// dereference media from an unauthorized room) so an image reaches a vision
	// model and a voice clip is transcribed by the ambient STT path.
	if allowed && strings.HasPrefix(ev.Content.URL, "mxc://") {
		if du := c.fetchMXC(ctx, ev.Content.URL, ev.Content.Info.MimeType); du != "" {
			if ev.Content.MsgType == "m.audio" {
				msg.Audio = []string{du}
			} else {
				msg.Images = []string{du}
			}
		}
	}
	c.emitInbound(msg, corr, allowed)

	if !allowed {
		// Fail-closed: a non-allowlisted room cannot drive the agent. Tell it once
		// so it isn't a silent black hole.
		_ = c.send(ctx, channel.Outbound{ChannelID: roomID, Text: "not authorized"}, "")
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
	_ = c.send(ctx, channel.Outbound{ChannelID: roomID, Text: reply, Attachments: rep.Attachments, Priority: channel.PriorityNotify}, corr)
}

// Send implements channel.Channel (used by the Pulse→Matrix sink and any
// out-of-band sender). Journaled under no correlation.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	return c.send(ctx, out, "")
}

// send PUTs m.room.message (chunked to the platform limit) and journals
// channel.outbound under corr.
func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	if strings.TrimSpace(out.Text) == "" && len(out.Attachments) == 0 {
		return nil // empty/whitespace is a no-op, not a failed send
	}
	for _, chunk := range channel.SplitText(out.Text, matrixMaxChars) {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		// A fresh transaction id per chunk makes the PUT idempotent (a retried
		// delivery with the same txn id is deduplicated by the homeserver).
		txn := ulid.New()
		endpoint := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
			c.base, url.PathEscape(out.ChannelID), url.PathEscape(txn))
		body, _ := json.Marshal(map[string]any{"msgtype": "m.text", "body": chunk})
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.token)
		resp, err := c.client.Do(req)
		if err != nil {
			return c.scrubToken(err)
		}
		err = func() error {
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				return fmt.Errorf("matrix send: status %d", resp.StatusCode)
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
			return nil
		}()
		if err != nil {
			return err
		}
	}
	for _, att := range out.Attachments {
		if err := c.sendMedia(ctx, out.ChannelID, att); err != nil {
			return err
		}
	}
	c.emitOutbound(out, corr)
	return nil
}

// fetchMXC downloads an mxc:// content URI from the media repo and returns it as
// an inline data: URL. Best-effort: returns "" on any failure. The mxc form is
// mxc://<server>/<mediaId>.
func (c *Channel) fetchMXC(ctx context.Context, mxc, mime string) string {
	rest := strings.TrimPrefix(mxc, "mxc://")
	server, mediaID, ok := strings.Cut(rest, "/")
	if !ok || server == "" || mediaID == "" {
		return ""
	}
	endpoint := fmt.Sprintf("%s/_matrix/media/v3/download/%s/%s",
		c.base, url.PathEscape(server), url.PathEscape(mediaID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil || len(data) == 0 || len(data) > 16<<20 {
		return ""
	}
	if mime == "" {
		mime = resp.Header.Get("Content-Type")
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// sendMedia uploads an attachment to the media repo, then posts an m.audio /
// m.image / m.file room event referencing the returned mxc:// URI.
