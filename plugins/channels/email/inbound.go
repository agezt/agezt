// SPDX-License-Identifier: MIT

package email

// Inbound email: poll a mailbox (IMAP or POP3) so the channel is two-way. New
// messages from allowlisted senders run the agent; the answer is sent back over
// SMTP as a reply. IMAP uses github.com/emersion/go-imap/v2 (SEARCH UNSEEN →
// FETCH → mark \Seen, so seen-state survives restarts); POP3 uses stdlib
// net/textproto with an in-memory UIDL seen-set primed at start (so a restart
// doesn't replay the whole mailbox). Message bodies are parsed with stdlib
// net/mail + mime; a multipart message yields its first text/plain part.
//
// Security: inbound mail is data, never kernel instructions. An Allowlist of
// sender addresses gates who may drive the agent (empty = fail-closed). Message
// ids are de-duplicated. Credentials are never logged.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

const (
	inboxPollDefault = 60 * time.Second
	maxInboxBytes    = 1 << 20 // bound a fetched message
	dedupCap         = 2048
)

// inboundMail is one fetched message, normalized.
type inboundMail struct {
	from      string // bare sender address (allowlist + reply target)
	subject   string
	body      string
	messageID string
}

// startInbound polls the configured mailbox until ctx is cancelled. Returns
// immediately (caller falls back to blocking) when no inbox is configured.
func (c *Channel) startInbound(ctx context.Context) bool {
	if c.inboxAddr == "" || c.handler == nil {
		return false
	}
	c.prime(ctx) // skip backlog: IMAP relies on \Seen; POP3 records current UIDLs
	t := time.NewTicker(c.pollEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return true
		case <-t.C:
			mails, err := c.poll(ctx)
			if err != nil {
				continue // transient; retry next tick
			}
			for _, m := range mails {
				c.dispatchInbound(ctx, m)
			}
		}
	}
}

func (c *Channel) prime(ctx context.Context) {
	if c.inboxProto == "pop3" {
		// Record the UIDLs already present so only mail arriving from now on is read.
		if uidls, err := c.popUIDLs(ctx); err == nil {
			c.smu.Lock()
			for _, u := range uidls {
				c.seen[u] = struct{}{}
			}
			c.smu.Unlock()
		}
	}
	// IMAP: nothing to prime — SEARCH UNSEEN + \Seen marking is the cursor.
}

func (c *Channel) poll(ctx context.Context) ([]inboundMail, error) {
	if c.inboxProto == "pop3" {
		return c.pollPOP3(ctx)
	}
	return c.pollIMAP(ctx)
}

func (c *Channel) dispatchInbound(ctx context.Context, m inboundMail) {
	if m.from == "" || strings.TrimSpace(m.body) == "" && strings.TrimSpace(m.subject) == "" {
		return
	}
	if m.messageID != "" && c.seenBefore("mid:"+m.messageID) {
		return
	}
	text := m.subject
	if strings.TrimSpace(m.body) != "" {
		if text != "" {
			text += "\n\n"
		}
		text += m.body
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "email",
		ChannelID:    m.from,
		Sender:       m.from,
		Text:         text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.allow.Allows(m.from)
	c.emitInbound(msg, corr, allowed)
	if !allowed {
		return
	}
	rep, err := c.handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	if strings.TrimSpace(rep.Text) == "" {
		return
	}
	// Reply over SMTP to the sender (who is allowlisted). Subject threads as Re:.
	_ = c.Send(ctx, channel.Outbound{
		ChannelID: m.from,
		Text:      rep.Text,
		Priority:  channel.PriorityNotify,
	})
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.email",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-email",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "email", "channel_id": msg.ChannelID,
			"sender": msg.Sender, "text": msg.Text, "allowed": allowed,
		},
	})
}

func (c *Channel) seenBefore(id string) bool {
	c.smu.Lock()
	defer c.smu.Unlock()
	if _, ok := c.seen[id]; ok {
		return true
	}
	c.seen[id] = struct{}{}
	c.ring = append(c.ring, id)
	if len(c.ring) > dedupCap {
		old := c.ring[0]
		c.ring = c.ring[1:]
		delete(c.seen, old)
	}
	return false
}

// --- IMAP -----------------------------------------------------------------

func (c *Channel) dialIMAP() (*imapclient.Client, error) {
	switch c.inboxTLS {
	case "starttls":
		return imapclient.DialStartTLS(c.inboxAddr, nil)
	case "none":
		return imapclient.DialInsecure(c.inboxAddr, nil)
	default:
		return imapclient.DialTLS(c.inboxAddr, nil)
	}
}

func (c *Channel) pollIMAP(_ context.Context) ([]inboundMail, error) {
	cl, err := c.dialIMAP()
	if err != nil {
		return nil, err
	}
	defer cl.Close()
	if err := cl.Login(c.inboxUser, c.inboxPass).Wait(); err != nil {
		return nil, err
	}
	defer cl.Logout().Wait()
	if _, err := cl.Select("INBOX", nil).Wait(); err != nil {
		return nil, err
	}
	data, err := cl.UIDSearch(&imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}, nil).Wait()
	if err != nil {
		return nil, err
	}
	uids := data.AllUIDs()
	if len(uids) == 0 {
		return nil, nil
	}
	set := imap.UIDSetNum(uids...)
	section := &imap.FetchItemBodySection{}
	msgs, err := cl.Fetch(set, &imap.FetchOptions{
		Envelope:    true,
		BodySection: []*imap.FetchItemBodySection{section},
	}).Collect()
	if err != nil {
		return nil, err
	}
	var out []inboundMail
	for _, mb := range msgs {
		raw := mb.FindBodySection(section)
		if len(raw) == 0 {
			continue
		}
		if len(raw) > maxInboxBytes {
			raw = raw[:maxInboxBytes]
		}
		if m, ok := parseMail(raw); ok {
			out = append(out, m)
		}
	}
	// Mark fetched messages \Seen so they aren't reprocessed next poll.
	_ = cl.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagSeen}, Silent: true}, nil).Close()
	return out, nil
}

// --- POP3 (stdlib net/textproto) ------------------------------------------

// popConn is a minimal POP3 session.
type popConn struct {
	conn net.Conn
	text *textproto.Conn
}

func (c *Channel) dialPOP3() (*popConn, error) {
	host := c.inboxAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	var conn net.Conn
	var err error
	switch c.inboxTLS {
	case "none", "starttls":
		conn, err = net.DialTimeout("tcp", c.inboxAddr, 15*time.Second)
	default: // implicit TLS (e.g. :995)
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", c.inboxAddr, &tls.Config{ServerName: host})
	}
	if err != nil {
		return nil, err
	}
	p := &popConn{conn: conn, text: textproto.NewConn(conn)}
	if _, err := p.readLine(); err != nil { // server greeting (+OK ...)
		p.close()
		return nil, err
	}
	// STARTTLS: upgrade the plaintext connection BEFORE sending credentials, so
	// USER/PASS never travel in the clear. (A bare "none" stays plaintext by the
	// operator's explicit choice; the default is implicit TLS above.)
	if c.inboxTLS == "starttls" {
		if _, err := p.cmd("STLS"); err != nil {
			p.close()
			return nil, err
		}
		tconn := tls.Client(conn, &tls.Config{ServerName: host})
		if err := tconn.Handshake(); err != nil {
			p.close()
			return nil, err
		}
		p.conn = tconn
		p.text = textproto.NewConn(tconn)
	}
	if _, err := p.cmd("USER " + c.inboxUser); err != nil {
		p.close()
		return nil, err
	}
	if _, err := p.cmd("PASS " + c.inboxPass); err != nil {
		p.close()
		return nil, err
	}
	return p, nil
}

func (p *popConn) readLine() (string, error) {
	line, err := p.text.ReadLine()
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(line, "-ERR") {
		return "", fmt.Errorf("pop3: %s", line)
	}
	return line, nil
}

// cmd sends a command and reads the single +OK/-ERR status line.
func (p *popConn) cmd(s string) (string, error) {
	if err := p.text.PrintfLine("%s", s); err != nil {
		return "", err
	}
	return p.readLine()
}

// readMultiline reads a dot-terminated multiline response after a +OK status.
func (p *popConn) readMultiline() ([]byte, error) {
	return p.text.ReadDotBytes()
}

func (p *popConn) close() {
	_ = p.text.PrintfLine("QUIT")
	_ = p.conn.Close()
}

// popUIDLs lists the UIDLs currently in the mailbox (for backlog priming).
func (c *Channel) popUIDLs(_ context.Context) ([]string, error) {
	p, err := c.dialPOP3()
	if err != nil {
		return nil, err
	}
	defer p.close()
	return p.uidls()
}

// uidls returns "msgnum uidl" → uidl list via the UIDL command.
func (p *popConn) uidls() ([]string, error) {
	if _, err := p.cmd("UIDL"); err != nil {
		return nil, err
	}
	body, err := p.readMultiline()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 {
			out = append(out, fields[1])
		}
	}
	return out, nil
}

func (c *Channel) pollPOP3(_ context.Context) ([]inboundMail, error) {
	p, err := c.dialPOP3()
	if err != nil {
		return nil, err
	}
	defer p.close()
	// Map msgnum → uidl.
	if _, err := p.cmd("UIDL"); err != nil {
		return nil, err
	}
	body, err := p.readMultiline()
	if err != nil {
		return nil, err
	}
	type slot struct{ num, uidl string }
	var slots []slot
	for _, line := range strings.Split(string(body), "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) == 2 {
			slots = append(slots, slot{f[0], f[1]})
		}
	}
	var out []inboundMail
	for _, sl := range slots {
		c.smu.Lock()
		_, seen := c.seen[sl.uidl]
		c.smu.Unlock()
		if seen {
			continue
		}
		// RETR the message, dot-stuffed multiline.
		if _, err := p.cmd("RETR " + sl.num); err != nil {
			continue
		}
		raw, err := p.readMultiline()
		if err != nil {
			continue
		}
		c.smu.Lock()
		c.seen[sl.uidl] = struct{}{}
		c.smu.Unlock()
		if len(raw) > maxInboxBytes {
			raw = raw[:maxInboxBytes]
		}
		if m, ok := parseMail(raw); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// --- RFC 5322 parsing (stdlib) --------------------------------------------

// parseMail extracts the sender, subject and a text body from a raw message.
func parseMail(raw []byte) (inboundMail, bool) {
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return inboundMail{}, false
	}
	dec := new(mime.WordDecoder)
	subject, _ := dec.DecodeHeader(m.Header.Get("Subject"))
	fromRaw := m.Header.Get("From")
	from := fromRaw
	if addr, perr := mail.ParseAddress(fromRaw); perr == nil {
		from = addr.Address
	}
	body := extractText(textproto.MIMEHeader(m.Header), m.Body)
	return inboundMail{
		from:      strings.TrimSpace(from),
		subject:   strings.TrimSpace(subject),
		body:      strings.TrimSpace(body),
		messageID: strings.TrimSpace(m.Header.Get("Message-Id")),
	}, true
}

// extractText returns a plain-text body: the first text/plain part of a multipart
// message, or the (CTE-decoded) body of a single-part message.
func extractText(header textproto.MIMEHeader, body io.Reader) string {
	ctype := header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(ctype)
	if err == nil && strings.HasPrefix(mediaType, "multipart/") && params["boundary"] != "" {
		mr := multipart.NewReader(body, params["boundary"])
		var firstAny string
		for {
			part, perr := mr.NextPart()
			if perr != nil {
				break
			}
			pt, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
			data := decodeBody(part.Header, part)
			if strings.HasPrefix(pt, "text/plain") {
				return data
			}
			if firstAny == "" && strings.HasPrefix(pt, "text/") {
				firstAny = data
			}
		}
		return firstAny
	}
	return decodeBody(header, body)
}

// decodeBody reads a body, decoding quoted-printable / base64 per the
// Content-Transfer-Encoding header.
func decodeBody(header textproto.MIMEHeader, r io.Reader) string {
	lr := io.LimitReader(r, maxInboxBytes)
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "quoted-printable":
		b, _ := io.ReadAll(quotedprintable.NewReader(lr))
		return string(b)
	case "base64":
		b, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, bufio.NewReader(lr)))
		return string(b)
	default:
		b, _ := io.ReadAll(lr)
		return string(b)
	}
}
