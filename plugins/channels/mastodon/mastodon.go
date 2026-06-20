// SPDX-License-Identifier: MIT

// Package mastodon is a two-way Mastodon channel. It polls the authenticated
// account's mention notifications and runs each as an inbound message, replying
// as a threaded status that @-mentions the sender. Outbound-only use (Pulse
// briefs, `agt send`) posts a standalone status. This upgrades the outbound-only
// push Mastodon entry to full duplex; when an allowlist is configured the
// dedicated channel owns the "mastodon" name and the push entry steps aside.
//
// Security (SPEC-04 §1.7): inbound mentions are data, never kernel instructions.
// An Allowlist of acct handles gates who may drive the agent (empty = fail-closed
// for driving, but still journaled). The API token rides an Authorization header;
// a since_id cursor + processing only new notifications guards against reprocessing.
// Status HTML is stripped to text before it reaches the agent.
package mastodon

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

const (
	maxChars        = 500 // Mastodon's default per-status limit
	defaultPollSecs = 60
)

// Config configures a Mastodon channel.
type Config struct {
	Server     string // instance base URL, e.g. https://mastodon.social (required)
	Token      string // access token (Bearer); needs read:notifications + write:statuses
	Allowlist  channel.Allowlist
	Bus        *bus.Bus
	Handler    channel.InboundHandler
	HTTPClient *http.Client
	PollSecs   int // mention poll interval; default 60
}

// Channel is the Mastodon surface.
type Channel struct {
	server   string
	token    string
	allow    channel.Allowlist
	bus      *bus.Bus
	handler  channel.InboundHandler
	client   *http.Client
	pollSecs int

	since string // notification id cursor
}

// New constructs a Mastodon channel, applying defaults.
func New(cfg Config) *Channel {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	poll := cfg.PollSecs
	if poll <= 0 {
		poll = defaultPollSecs
	}
	return &Channel{
		server:   strings.TrimRight(strings.TrimSpace(cfg.Server), "/"),
		token:    strings.TrimSpace(cfg.Token),
		allow:    cfg.Allowlist,
		bus:      cfg.Bus,
		handler:  cfg.Handler,
		client:   client,
		pollSecs: poll,
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "mastodon" }

// Start primes the notification cursor (skipping backlog) then polls for new
// mentions until ctx is cancelled. With no handler it still blocks so the
// daemon's lifecycle is uniform (outbound-only).
func (c *Channel) Start(ctx context.Context) error {
	if c.server == "" || c.token == "" || c.handler == nil {
		<-ctx.Done()
		return nil
	}
	c.prime(ctx) // best-effort: establishes since so we don't replay old mentions
	interval := time.Duration(c.pollSecs) * time.Second
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			c.poll(ctx)
		}
	}
}

// prime sets the cursor to the newest mention so Start only processes new ones.
func (c *Channel) prime(ctx context.Context) {
	ns, _ := c.fetchMentions(ctx, "")
	for _, n := range ns {
		if n.ID > c.since {
			c.since = n.ID
		}
	}
}

func (c *Channel) poll(ctx context.Context) {
	ns, err := c.fetchMentions(ctx, c.since)
	if err != nil {
		return
	}
	// The API returns newest-first; process oldest-first so the cursor advances
	// monotonically and replies are chronological.
	for i := len(ns) - 1; i >= 0; i-- {
		n := ns[i]
		if n.ID > c.since {
			c.since = n.ID
		}
		c.dispatch(ctx, n)
	}
}

func (c *Channel) dispatch(ctx context.Context, n notification) {
	acct := strings.TrimSpace(n.Status.Account.Acct)
	text := stripHTML(n.Status.Content)
	if acct == "" || text == "" {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "mastodon",
		ChannelID:    n.Status.ID, // reply target (in_reply_to_id)
		Sender:       acct,
		Text:         text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(acct)
	c.emitInbound(msg, corr, allowed)
	if !allowed || c.handler == nil {
		return
	}
	rep, err := c.handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	reply := rep.Text
	if reply == "" {
		return
	}
	vis := n.Status.Visibility
	if vis == "" {
		vis = "unlisted"
	}
	// Thread the reply and @-mention the sender so they're notified.
	_ = c.postStatus(ctx, "@"+acct+" "+reply, n.Status.ID, vis, corr)
}

// Send implements channel.Channel: post out.Text as a status. If out.ChannelID is
// a status id, the post is threaded as a reply to it; otherwise it's standalone
// (Pulse briefs).
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	text := strings.TrimSpace(out.Text)
	if text == "" {
		return nil
	}
	return c.postStatus(ctx, text, strings.TrimSpace(out.ChannelID), "unlisted", "chan-"+ulid.New())
}

func (c *Channel) postStatus(ctx context.Context, text, inReplyTo, visibility, corr string) error {
	if c.server == "" || c.token == "" {
		return fmt.Errorf("mastodon: server and token required")
	}
	chunks := channel.SplitText(text, maxChars)
	reply := inReplyTo
	for _, chunk := range chunks {
		form := url.Values{}
		form.Set("status", chunk)
		if visibility != "" {
			form.Set("visibility", visibility)
		}
		if reply != "" {
			form.Set("in_reply_to_id", reply)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.server+"/api/v1/statuses", strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Authorization", "Bearer "+c.token)
		resp, err := c.client.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("mastodon: post returned status %d", resp.StatusCode)
		}
		// Chain subsequent chunks as replies to the previous post id so a long
		// answer reads as a self-thread.
		var posted struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(body, &posted) == nil && posted.ID != "" {
			reply = posted.ID
		}
	}
	c.emitOutbound(channel.Outbound{ChannelID: inReplyTo, Text: text, Priority: channel.PriorityNotify}, corr)
	return nil
}

// fetchMentions GETs mention notifications newer than since (empty → newest page).
func (c *Channel) fetchMentions(ctx context.Context, since string) ([]notification, error) {
	u := c.server + "/api/v1/notifications?types[]=mention&limit=20"
	if since != "" {
		u += "&since_id=" + url.QueryEscape(since)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("mastodon: notifications status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var ns []notification
	if err := json.Unmarshal(body, &ns); err != nil {
		return nil, err
	}
	return ns, nil
}

// --- events ---------------------------------------------------------------

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.mastodon",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-mastodon",
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
		Subject:       "channel.outbound.mastodon",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-mastodon",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_id": out.ChannelID,
			"text":       out.Text,
			"priority":   string(out.Priority),
		},
	})
}

// --- wire shapes ----------------------------------------------------------

type notification struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status struct {
		ID         string `json:"id"`
		Content    string `json:"content"` // HTML
		Visibility string `json:"visibility"`
		Account    struct {
			Acct string `json:"acct"`
		} `json:"account"`
	} `json:"status"`
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// stripHTML turns Mastodon status HTML into plain text: <br> and </p> become
// line breaks, remaining tags are dropped, and HTML entities are unescaped.
func stripHTML(s string) string {
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "</p>", "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}
