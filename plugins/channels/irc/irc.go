// SPDX-License-Identifier: MIT

// Package irc is a two-way IRC channel (SPEC-04 §1): the daemon connects to an
// IRC server, joins channels, and bridges messages to the agent. Inbound
// PRIVMSGs from allowlisted sources drive the agent and its reply is sent back
// to the originating channel (or the sender, for a direct message); Pulse briefs
// and `agt send` post to the configured channels. Classic IRC — no API keys, no
// webhooks — so it works against any ircd (Libera.Chat, OFTC, a private server).
//
// It reconnects with backoff until ctx is cancelled. An empty allowlist is
// fail-closed (outbound-only): the bot can post but won't act on inbound until a
// source is allowlisted. Lines are length-clamped to the IRC 512-byte limit.
package irc

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

// Config configures the IRC channel.
type Config struct {
	Kind      string   // channel kind for labelling/journaling (default "irc"); e.g. "twitch"
	Server    string   // host:port (e.g. irc.libera.chat:6697)
	TLS       bool     // dial with TLS (typical for :6697)
	Nick      string   // the bot's nick (also used as username/realname)
	Password  string   // optional server password (PASS)
	Channels  []string // channels to join, e.g. ["#agezt"]
	Allowlist channel.Allowlist
	Bus       *bus.Bus
	Handler   channel.InboundHandler
}

// Channel is the IRC messaging surface.
type Channel struct {
	cfg  Config
	mu   sync.Mutex // guards conn for concurrent writes
	conn net.Conn
}

// New constructs an IRC channel from cfg.
func New(cfg Config) *Channel { return &Channel{cfg: cfg} }

// kind is the channel kind used for labelling and journaling — "irc" by default,
// or an override like "twitch" for an IRC-protocol service.
func (c *Channel) kind() string {
	if c.cfg.Kind != "" {
		return c.cfg.Kind
	}
	return "irc"
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return c.kind() }

// Start connects, registers, joins, and runs the read loop, reconnecting with
// backoff until ctx is cancelled.
func (c *Channel) Start(ctx context.Context) error {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := c.session(ctx); err != nil && ctx.Err() == nil {
			// Transient: wait (bounded) then reconnect.
			t := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				t.Stop()
				return nil
			case <-t.C:
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		if ctx.Err() != nil {
			return nil
		}
	}
}

// session dials, registers, and reads until the connection drops or ctx ends.
func (c *Channel) session(ctx context.Context) error {
	var conn net.Conn
	var err error
	dialer := &net.Dialer{Timeout: 20 * time.Second}
	if c.cfg.TLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", c.cfg.Server, nil)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", c.cfg.Server)
	}
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.mu.Unlock()
		_ = conn.Close()
	}()

	// Close the conn when ctx is cancelled so the blocking Read returns.
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	if c.cfg.Password != "" {
		c.writeLine("PASS " + c.cfg.Password)
	}
	c.writeLine("NICK " + c.cfg.Nick)
	c.writeLine(fmt.Sprintf("USER %s 0 * :%s", c.cfg.Nick, c.cfg.Nick))

	r := bufio.NewReaderSize(conn, 4096)
	for {
		line, rerr := r.ReadString('\n')
		if rerr != nil {
			return rerr
		}
		c.handleLine(ctx, strings.TrimRight(line, "\r\n"))
	}
}

// handleLine processes one raw IRC line: PING keepalive, welcome→JOIN, PRIVMSG→agent.
func (c *Channel) handleLine(ctx context.Context, line string) {
	if line == "" {
		return
	}
	if strings.HasPrefix(line, "PING ") {
		c.writeLine("PONG " + line[len("PING "):])
		return
	}
	prefix, cmd, params := parseLine(line)
	switch cmd {
	case "001": // RPL_WELCOME — registered; safe to join.
		for _, ch := range c.cfg.Channels {
			c.writeLine("JOIN " + ch)
		}
	case "PRIVMSG":
		c.handlePrivmsg(ctx, prefix, params)
	}
}

// handlePrivmsg turns an inbound PRIVMSG into a UnifiedMessage, gates it on the
// allowlist, runs the agent, and sends the reply back to the right place.
func (c *Channel) handlePrivmsg(ctx context.Context, prefix string, params string) {
	target, text, ok := splitPrivmsg(params)
	if !ok || text == "" {
		return
	}
	sender := nickOf(prefix)
	// A channel message replies to the channel; a DM (target == our nick)
	// replies to the sender. The allowlist + history key is that reply target.
	replyTo := target
	if !strings.HasPrefix(target, "#") {
		replyTo = sender
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  c.kind(),
		ChannelID:    replyTo,
		Sender:       sender,
		Text:         text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.cfg.Allowlist.Allows(replyTo)
	c.emitInbound(msg, corr, allowed)
	if !allowed || c.cfg.Handler == nil {
		return
	}
	rep, err := c.cfg.Handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	reply := rep.Text
	if reply == "" {
		return
	}
	_ = c.Send(ctx, channel.Outbound{ChannelID: replyTo, Text: reply, Priority: channel.PriorityNotify})
}

// Send posts out.Text to out.ChannelID (a #channel or a nick) as PRIVMSG lines.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	target := strings.TrimSpace(out.ChannelID)
	text := strings.TrimSpace(out.Text)
	if target == "" {
		return fmt.Errorf("irc: send requires a target channel/nick")
	}
	if text == "" {
		return nil
	}
	c.mu.Lock()
	connected := c.conn != nil
	c.mu.Unlock()
	if !connected {
		return fmt.Errorf("irc: not connected")
	}
	for _, line := range splitLines(text) {
		c.writeLine("PRIVMSG " + target + " :" + line)
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel.outbound." + c.kind(), Kind: event.KindChannelOutbound, Actor: "channel-" + c.kind(),
			Payload: map[string]any{"channel_kind": c.kind(), "channel_id": target, "text": text},
		})
	}
	return nil
}

// writeLine writes one CRLF-terminated IRC line under the conn mutex.
func (c *Channel) writeLine(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return
	}
	if len(s) > 480 { // leave headroom under the 512-byte limit (incl CRLF + prefix)
		s = s[:480]
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(20 * time.Second))
	_, _ = c.conn.Write([]byte(s + "\r\n"))
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound." + c.kind(),
		Kind:          event.KindChannelInbound,
		Actor:         "channel-" + c.kind(),
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": c.kind(), "channel_id": msg.ChannelID,
			"sender": msg.Sender, "text": msg.Text, "allowed": allowed,
		},
	})
}

// parseLine splits an IRC line into prefix (without leading ':'), command, and
// the remaining params string.
func parseLine(line string) (prefix, cmd, params string) {
	if strings.HasPrefix(line, ":") {
		sp := strings.IndexByte(line, ' ')
		if sp < 0 {
			return line[1:], "", ""
		}
		prefix = line[1:sp]
		line = line[sp+1:]
	}
	sp := strings.IndexByte(line, ' ')
	if sp < 0 {
		return prefix, line, ""
	}
	return prefix, line[:sp], line[sp+1:]
}

// splitPrivmsg parses "target :message" (PRIVMSG params) into target + message.
func splitPrivmsg(params string) (target, message string, ok bool) {
	sp := strings.IndexByte(params, ' ')
	if sp < 0 {
		return "", "", false
	}
	target = params[:sp]
	rest := params[sp+1:]
	message = strings.TrimPrefix(rest, ":")
	return target, message, true
}

// nickOf extracts the nick from an IRC prefix "nick!user@host".
func nickOf(prefix string) string {
	if i := strings.IndexByte(prefix, '!'); i >= 0 {
		return prefix[:i]
	}
	return prefix
}

// splitLines breaks text on newlines and clamps each to a safe IRC payload size,
// so a multi-line agent reply becomes several PRIVMSGs.
func splitLines(text string) []string {
	var out []string
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimRight(ln, "\r")
		if ln == "" {
			continue
		}
		for len(ln) > 400 {
			out = append(out, ln[:400])
			ln = ln[400:]
		}
		out = append(out, ln)
	}
	return out
}
