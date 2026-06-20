// SPDX-License-Identifier: MIT

package email

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestParseMail(t *testing.T) {
	// Plain text.
	plain := "From: Alice <alice@example.com>\r\nSubject: hi there\r\nMessage-Id: <1@x>\r\n\r\nhello body\r\n"
	m, ok := parseMail([]byte(plain))
	if !ok || m.from != "alice@example.com" || m.subject != "hi there" || m.body != "hello body" || m.messageID != "<1@x>" {
		t.Fatalf("plain = %+v ok=%v", m, ok)
	}

	// Quoted-printable single part.
	qp := "From: b@x.com\r\nSubject: q\r\nContent-Type: text/plain\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nhi=20there=3D\r\n"
	m, ok = parseMail([]byte(qp))
	if !ok || !strings.Contains(m.body, "hi there=") {
		t.Fatalf("qp body = %q", m.body)
	}

	// Multipart/alternative → first text/plain part.
	mp := "From: c@x.com\r\nSubject: mp\r\nContent-Type: multipart/alternative; boundary=BB\r\n\r\n" +
		"--BB\r\nContent-Type: text/plain\r\n\r\nplain part\r\n" +
		"--BB\r\nContent-Type: text/html\r\n\r\n<p>html part</p>\r\n--BB--\r\n"
	m, ok = parseMail([]byte(mp))
	if !ok || m.body != "plain part" {
		t.Fatalf("multipart body = %q", m.body)
	}
}

// fakePOP serves a single hard-coded message over a minimal POP3 dialogue.
func fakePOP(t *testing.T, uidl, raw string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				w := conn
				fmt.Fprintf(w, "+OK ready\r\n")
				sc := bufio.NewScanner(conn)
				for sc.Scan() {
					cmd := strings.TrimSpace(sc.Text())
					up := strings.ToUpper(cmd)
					switch {
					case strings.HasPrefix(up, "USER"), strings.HasPrefix(up, "PASS"):
						fmt.Fprintf(w, "+OK\r\n")
					case up == "UIDL":
						fmt.Fprintf(w, "+OK\r\n1 %s\r\n.\r\n", uidl)
					case strings.HasPrefix(up, "RETR"):
						fmt.Fprintf(w, "+OK\r\n%s\r\n.\r\n", strings.TrimRight(raw, "\r\n"))
					case up == "QUIT":
						fmt.Fprintf(w, "+OK bye\r\n")
						return
					default:
						fmt.Fprintf(w, "+OK\r\n")
					}
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func TestPOP3InboundDispatchesAndReplies(t *testing.T) {
	raw := "From: boss@example.com\r\nSubject: status?\r\nMessage-Id: <m1@x>\r\n\r\nwhat's the status\r\n"
	addr := fakePOP(t, "uid-1", raw)

	var seen string
	var mu sync.Mutex
	var sentTo, sentBody string
	fakeSend := func(_ string, _ smtp.Auth, _ string, to []string, msg []byte) error {
		mu.Lock()
		defer mu.Unlock()
		if len(to) > 0 {
			sentTo = to[0]
		}
		sentBody = string(msg)
		return nil
	}

	c := New(Config{
		Addr:          "smtp.example.com:587",
		From:          "agent@example.com",
		Allowlist:     channel.NewAllowlist([]string{"boss@example.com"}),
		Send:          fakeSend,
		InboxAddr:     addr,
		InboxProtocol: "pop3",
		InboxTLS:      "none",
		InboxUsername: "u",
		InboxPassword: "p",
		Handler: func(_ context.Context, m channel.UnifiedMessage, _ string) (channel.Reply, error) {
			seen = m.Text
			return channel.Reply{Text: "all good"}, nil
		},
	})

	mails, err := c.pollPOP3(context.Background())
	if err != nil {
		t.Fatalf("pollPOP3: %v", err)
	}
	if len(mails) != 1 || mails[0].from != "boss@example.com" {
		t.Fatalf("mails = %+v", mails)
	}
	c.dispatchInbound(context.Background(), mails[0])

	if !strings.Contains(seen, "status?") || !strings.Contains(seen, "what's the status") {
		t.Fatalf("handler saw %q", seen)
	}
	mu.Lock()
	defer mu.Unlock()
	if sentTo != "boss@example.com" || !strings.Contains(sentBody, "all good") {
		t.Fatalf("reply to=%q body=%q", sentTo, sentBody)
	}
}

func TestPOP3SkipsAllowlistMiss(t *testing.T) {
	raw := "From: stranger@evil.com\r\nSubject: hi\r\nMessage-Id: <m2@x>\r\n\r\nlet me in\r\n"
	addr := fakePOP(t, "uid-2", raw)
	called := false
	c := New(Config{
		Addr:          "smtp.example.com:587",
		From:          "agent@example.com",
		Allowlist:     channel.NewAllowlist([]string{"boss@example.com"}),
		Send:          func(string, smtp.Auth, string, []string, []byte) error { return nil },
		InboxAddr:     addr,
		InboxProtocol: "pop3",
		InboxTLS:      "none",
		Handler: func(context.Context, channel.UnifiedMessage, string) (channel.Reply, error) {
			called = true
			return channel.Reply{Text: "x"}, nil
		},
	})
	mails, err := c.pollPOP3(context.Background())
	if err != nil || len(mails) != 1 {
		t.Fatalf("pollPOP3 = %+v %v", mails, err)
	}
	c.dispatchInbound(context.Background(), mails[0])
	if called {
		t.Fatal("non-allowlisted sender must not reach the handler")
	}
}
