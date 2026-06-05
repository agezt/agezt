// SPDX-License-Identifier: MIT

// Package email is an outbound email channel (SPEC-04 §1): it delivers Agezt
// messages — Pulse briefs and `agt send` — to operator inboxes over SMTP. It is
// outbound-only: receiving mail needs an IMAP/POP poller or an inbound MX, which
// depend on a live mailbox and are out of scope here; Start blocks until ctx is
// cancelled so the daemon's channel lifecycle stays uniform.
//
// The "channel_id" of an outbound message is the recipient email address; an
// Allowlist restricts which addresses Agezt will mail (so a misconfigured brief
// can't spray arbitrary recipients). Transport is stdlib net/smtp (no new
// dependency); the send function is injectable so the message construction is
// unit-testable without a live SMTP server.
//
// Security (SPEC-04 §1.7): outbound only — there's no inbound injection surface.
// Credentials (SMTP username/password) are never logged; the Allowlist is
// fail-closed (empty → sends to nobody).
package email

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

// SendFunc is the SMTP transport seam. The default is net/smtp.SendMail; tests
// inject a fake to capture the envelope + RFC 5322 message without a live server.
type SendFunc func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error

// Config configures the outbound email channel.
type Config struct {
	// Addr is the SMTP server "host:port" (e.g. "smtp.example.com:587").
	Addr string
	// Username / Password authenticate via SMTP AUTH PLAIN when both are set;
	// empty → no auth (e.g. a local relay).
	Username string
	Password string
	// From is the envelope + header sender address.
	From string
	// Allowlist restricts which recipient addresses may be mailed (fail-closed).
	Allowlist channel.Allowlist
	// Bus journals channel.outbound events. May be nil.
	Bus *bus.Bus
	// Send overrides the SMTP transport (tests); nil → net/smtp.SendMail.
	Send SendFunc
	// now overrides the clock for the Date header (tests); nil → time.Now.
	now func() time.Time
}

// Channel is the outbound email messaging surface.
type Channel struct {
	addr  string
	from  string
	auth  smtp.Auth
	allow channel.Allowlist
	bus   *bus.Bus
	send  SendFunc
	now   func() time.Time
}

// New constructs an outbound email Channel.
func New(cfg Config) *Channel {
	send := cfg.Send
	if send == nil {
		send = smtp.SendMail
	}
	now := cfg.now
	if now == nil {
		now = time.Now
	}
	var auth smtp.Auth
	if cfg.Username != "" && cfg.Password != "" {
		host := cfg.Addr
		if i := strings.LastIndex(host, ":"); i >= 0 {
			host = host[:i]
		}
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, host)
	}
	return &Channel{
		addr:  cfg.Addr,
		from:  cfg.From,
		auth:  auth,
		allow: cfg.Allowlist,
		bus:   cfg.Bus,
		send:  send,
		now:   now,
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "email" }

// Start implements channel.Channel. Email is outbound-only, so Start just blocks
// until ctx is cancelled (keeping the daemon's per-channel lifecycle uniform).
func (c *Channel) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// Send delivers an outbound message as an email to out.ChannelID (the recipient
// address), subject to the Allowlist. Errors when the recipient isn't allowed,
// no transport address is configured, or SMTP fails.
func (c *Channel) Send(_ context.Context, out channel.Outbound) error {
	to := strings.TrimSpace(out.ChannelID)
	if to == "" {
		return fmt.Errorf("email: recipient (channel_id) required")
	}
	if !c.allow.Allows(to) {
		return fmt.Errorf("email: recipient %q not in allowlist", to)
	}
	if c.addr == "" {
		return fmt.Errorf("email: no SMTP address configured")
	}
	msg := c.buildMessage(to, out)
	if err := c.send(c.addr, c.auth, c.from, []string{to}, msg); err != nil {
		return fmt.Errorf("email: send: %w", err)
	}
	c.emitOutbound(out)
	return nil
}

// buildMessage renders an RFC 5322 message. The subject is derived from the
// priority and the first line of the body; the body is sent as text/plain UTF-8.
func (c *Channel) buildMessage(to string, out channel.Outbound) []byte {
	subject := subjectFor(out)
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", c.from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "Date: %s\r\n", c.now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	// Normalize bare LF to CRLF for SMTP DATA.
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(out.Text, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(b.String())
}

// subjectFor derives a one-line subject: an urgency prefix plus the first line of
// the body, length-bounded so a long body can't bloat the header.
func subjectFor(out channel.Outbound) string {
	prefix := "Agezt"
	switch out.Priority {
	case channel.PriorityUrgent:
		prefix = "Agezt [urgent]"
	case channel.PriorityNotify:
		prefix = "Agezt [notify]"
	}
	// Cut at the first CR OR LF: the subject is the first line, and a bare CR left
	// in the header would be a header-injection vector against lenient MTAs (a lone
	// '\n'-only cut let an interior '\r' survive into the Subject line). (M479)
	firstLine := out.Text
	if i := strings.IndexAny(firstLine, "\r\n"); i >= 0 {
		firstLine = firstLine[:i]
	}
	firstLine = strings.TrimSpace(firstLine)
	if firstLine == "" {
		return prefix
	}
	const max = 120
	if len(firstLine) > max {
		firstLine = firstLine[:max] + "…"
	}
	return prefix + ": " + firstLine
}

func (c *Channel) emitOutbound(out channel.Outbound) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.outbound.email",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-email",
		CorrelationID: "chan-" + ulid.New(),
		Payload: map[string]any{
			"channel_id": out.ChannelID,
			"text":       out.Text,
			"priority":   string(out.Priority),
		},
	})
}
