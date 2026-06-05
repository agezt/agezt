// SPDX-License-Identifier: MIT

package email

import (
	"bufio"
	"context"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

// capture records the last SMTP send for assertions.
type capture struct {
	addr string
	from string
	to   []string
	msg  string
	hits int
}

func newChannel(t *testing.T, cap *capture, allow ...string) *Channel {
	t.Helper()
	return New(Config{
		Addr:      "smtp.test:25",
		From:      "agezt@example.com",
		Allowlist: channel.NewAllowlist(allow),
		Send: func(addr string, _ smtp.Auth, from string, to []string, msg []byte) error {
			cap.addr, cap.from, cap.to, cap.msg = addr, from, to, string(msg)
			cap.hits++
			return nil
		},
	})
}

// TestSend_BuildsMessageAndDelivers (M335): an allowlisted recipient gets a
// well-formed RFC 5322 message with the right envelope, headers, and subject.
func TestSend_BuildsMessageAndDelivers(t *testing.T) {
	var cap capture
	c := newChannel(t, &cap, "ops@example.com")
	err := c.Send(context.Background(), channel.Outbound{
		ChannelID: "ops@example.com",
		Text:      "Disk is 92% full\nact soon",
		Priority:  channel.PriorityUrgent,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if cap.addr != "smtp.test:25" || cap.from != "agezt@example.com" {
		t.Errorf("envelope addr=%q from=%q", cap.addr, cap.from)
	}
	if len(cap.to) != 1 || cap.to[0] != "ops@example.com" {
		t.Errorf("rcpt=%v", cap.to)
	}
	for _, want := range []string{
		"From: agezt@example.com\r\n",
		"To: ops@example.com\r\n",
		"Subject: Agezt [urgent]: Disk is 92% full\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
		"\r\n\r\nDisk is 92% full\r\nact soon", // headers end, CRLF body
	} {
		if !strings.Contains(cap.msg, want) {
			t.Errorf("message missing %q in:\n%q", want, cap.msg)
		}
	}
}

// TestSend_AllowlistGates: a recipient not on the allowlist is rejected and no
// mail is sent (fail-closed — a misconfigured brief can't spray arbitrary addrs).
func TestSend_AllowlistGates(t *testing.T) {
	var cap capture
	c := newChannel(t, &cap, "ops@example.com")
	err := c.Send(context.Background(), channel.Outbound{ChannelID: "stranger@evil.com", Text: "hi"})
	if err == nil {
		t.Fatal("expected allowlist rejection")
	}
	if cap.hits != 0 {
		t.Errorf("no mail should be sent to a non-allowlisted address (hits=%d)", cap.hits)
	}
}

// TestSend_RequiresRecipientAndAddr covers the empty-recipient and no-server
// guards.
func TestSend_RequiresRecipientAndAddr(t *testing.T) {
	var cap capture
	c := newChannel(t, &cap, "ops@example.com")
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "", Text: "x"}); err == nil {
		t.Error("empty recipient should error")
	}
	// No SMTP addr configured.
	c2 := New(Config{Allowlist: channel.NewAllowlist([]string{"ops@example.com"}),
		Send: func(string, smtp.Auth, string, []string, []byte) error { return nil }})
	if err := c2.Send(context.Background(), channel.Outbound{ChannelID: "ops@example.com", Text: "x"}); err == nil {
		t.Error("no SMTP addr should error")
	}
}

// TestSubjectFor covers the priority prefix, first-line extraction, and truncation.
func TestSubjectFor(t *testing.T) {
	cases := []struct{ text, prio, want string }{
		{"hello world", "", "Agezt: hello world"},
		{"line1\nline2", string(channel.PriorityNotify), "Agezt [notify]: line1"},
		{"   \n", "", "Agezt"},
		// A bare CR must not survive into the Subject header (injection guard, M479).
		{"Hello\rBcc: evil@example.com", "", "Agezt: Hello"},
		{"first\r\nsecond", "", "Agezt: first"},
	}
	for _, c := range cases {
		got := subjectFor(channel.Outbound{Text: c.text, Priority: channel.Priority(c.prio)})
		if got != c.want {
			t.Errorf("subjectFor(%q,%q)=%q want %q", c.text, c.prio, got, c.want)
		}
	}
	long := subjectFor(channel.Outbound{Text: strings.Repeat("a", 300)})
	if !strings.HasSuffix(long, "…") {
		t.Errorf("long subject should be truncated: %q", long)
	}
}

// TestSend_RealSMTPTransport drives the actual net/smtp.SendMail path against a
// minimal in-process SMTP server (no auth, no TLS), proving the real transport —
// not just the injected seam — delivers the message.
func TestSend_RealSMTPTransport(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var (
		mu      sync.Mutex
		gotData string
		gotRcpt string
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := bufio.NewWriter(conn)
		writeLine := func(s string) { w.WriteString(s + "\r\n"); w.Flush() }
		writeLine("220 mock ESMTP")
		inData := false
		var data strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			trimmed := strings.TrimRight(line, "\r\n")
			if inData {
				if trimmed == "." {
					inData = false
					mu.Lock()
					gotData = data.String()
					mu.Unlock()
					writeLine("250 OK queued")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch {
			case strings.HasPrefix(trimmed, "EHLO"), strings.HasPrefix(trimmed, "HELO"):
				writeLine("250 mock")
			case strings.HasPrefix(trimmed, "MAIL FROM"):
				writeLine("250 OK")
			case strings.HasPrefix(trimmed, "RCPT TO"):
				mu.Lock()
				gotRcpt = trimmed
				mu.Unlock()
				writeLine("250 OK")
			case strings.HasPrefix(trimmed, "DATA"):
				writeLine("354 end with .")
				inData = true
			case strings.HasPrefix(trimmed, "QUIT"):
				writeLine("221 bye")
				return
			default:
				writeLine("250 OK")
			}
		}
	}()

	c := New(Config{
		Addr:      ln.Addr().String(),
		From:      "agezt@example.com",
		Allowlist: channel.NewAllowlist([]string{"ops@example.com"}),
	})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "ops@example.com", Text: "real delivery", Priority: channel.PriorityNotify}); err != nil {
		t.Fatalf("Send over real SMTP: %v", err)
	}
	<-done

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(gotRcpt, "ops@example.com") {
		t.Errorf("server RCPT=%q", gotRcpt)
	}
	if !strings.Contains(gotData, "Subject: Agezt [notify]: real delivery") || !strings.Contains(gotData, "real delivery") {
		t.Errorf("server received DATA=%q", gotData)
	}
}
