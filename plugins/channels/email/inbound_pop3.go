// SPDX-License-Identifier: MIT

// Email channel: POP3 transport (popConn + dialPOP3 + readLine/cmd/readMultiline/close + popUIDLs/uidls + pollPOP3).
// Code extracted from inbound.go during the Day-125 god-file split.
// Public API unchanged.
package email


import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"strings"
	"time"

	"crypto/tls"
	"encoding/base64"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
)

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
