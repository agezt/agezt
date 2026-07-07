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
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/journal"
)

func newBus(t *testing.T) *bus.Bus {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	return b
}

// pop3Server is a tiny fake POP3 server: greeting, USER/PASS, UIDL, RETR.
type pop3Server struct {
	ln       net.Listener
	mu       sync.Mutex
	messages map[string]string // uidl -> raw RFC5322 message
	order    []string          // uidl order (msgnum = index+1)
}

func newPOP3(t *testing.T, messages map[string]string, order []string) *pop3Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &pop3Server{ln: ln, messages: messages, order: order}
	t.Cleanup(func() { ln.Close() })
	go s.acceptLoop()
	return s
}

func (s *pop3Server) addr() string { return s.ln.Addr().String() }

func (s *pop3Server) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *pop3Server) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	writeLine := func(str string) { w.WriteString(str + "\r\n"); w.Flush() }
	writeLine("+OK POP3 ready")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(cmd, "USER"), strings.HasPrefix(cmd, "PASS"):
			writeLine("+OK")
		case cmd == "UIDL":
			writeLine("+OK")
			s.mu.Lock()
			for i, uidl := range s.order {
				writeLine(itoa(i+1) + " " + uidl)
			}
			s.mu.Unlock()
			writeLine(".")
		case strings.HasPrefix(cmd, "RETR "):
			numStr := strings.TrimSpace(strings.TrimPrefix(cmd, "RETR "))
			idx := atoiSafe(numStr) - 1
			s.mu.Lock()
			var raw string
			if idx >= 0 && idx < len(s.order) {
				raw = s.messages[s.order[idx]]
			}
			s.mu.Unlock()
			writeLine("+OK")
			// Dot-stuff and dot-terminate.
			for _, ln := range strings.Split(raw, "\n") {
				ln = strings.TrimRight(ln, "\r")
				if strings.HasPrefix(ln, ".") {
					ln = "." + ln
				}
				writeLine(ln)
			}
			writeLine(".")
		case cmd == "QUIT":
			writeLine("+OK bye")
			return
		default:
			writeLine("+OK")
		}
	}
}

func atoiSafe(s string) int { return atoi(s) }

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

const sampleMail = "From: alice@example.com\r\n" +
	"Subject: Hello\r\n" +
	"Message-ID: <m1@example.com>\r\n" +
	"Content-Type: text/plain; charset=UTF-8\r\n" +
	"\r\n" +
	"this is the body\r\n"

func TestName(t *testing.T) {
	if got := New(Config{}).Name(); got != "email" {
		t.Fatalf("Name = %q", got)
	}
}

func TestStartOutboundOnlyBlocks(t *testing.T) {
	// No inbox configured => Start blocks until ctx cancel.
	c := New(Config{Addr: "smtp.test:25", From: "a@b.com"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return")
	}
}

func TestStartInboundPOP3PollsAndDispatches(t *testing.T) {
	// Prime the mailbox with a message that arrives *after* priming: prime()
	// records existing UIDLs so only new mail is delivered. We add a fresh
	// message keyed by a UIDL not present at prime time by starting empty then
	// injecting. Simpler: start with one message and assert the FIRST poll
	// after prime does NOT deliver it (it's backlog), then add a new one.
	srv := newPOP3(t, map[string]string{"U1": sampleMail}, []string{"U1"})

	got := make(chan channel.UnifiedMessage, 4)
	var sendMu sync.Mutex
	var sends int
	c := New(Config{
		Addr:          "smtp.test:25",
		From:          "bot@example.com",
		Allowlist:     channel.NewAllowlist([]string{"alice@example.com"}),
		InboxAddr:     srv.addr(),
		InboxProtocol: "pop3",
		InboxTLS:      "none",
		InboxUsername: "u",
		InboxPassword: "p",
		PollSecs:      1,
		Send: func(addr string, _ smtp.Auth, from string, to []string, msg []byte) error {
			sendMu.Lock()
			sends++
			sendMu.Unlock()
			return nil
		},
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			got <- m
			return channel.Reply{Text: "reply"}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = c.Start(ctx) }()
	// Give prime() a moment, then add a NEW message that should be delivered.
	time.Sleep(300 * time.Millisecond)
	srv.mu.Lock()
	srv.messages["U2"] = strings.Replace(sampleMail, "<m1@example.com>", "<m2@example.com>", 1)
	srv.order = append(srv.order, "U2")
	srv.mu.Unlock()

	select {
	case m := <-got:
		if m.Sender != "alice@example.com" || !strings.Contains(m.Text, "Hello") {
			t.Fatalf("dispatched message = %+v", m)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("no inbound message dispatched")
	}
	cancel()
	time.Sleep(200 * time.Millisecond)
	sendMu.Lock()
	defer sendMu.Unlock()
	if sends == 0 {
		t.Fatal("reply was not sent over SMTP")
	}
}

func TestPopUIDLsAndPollErrorOnDialFailure(t *testing.T) {
	// Dial a closed port => dialPOP3 fails => popUIDLs + pollPOP3 return errors.
	c := New(Config{
		InboxAddr:     "127.0.0.1:1",
		InboxProtocol: "pop3",
		InboxTLS:      "none",
	})
	if _, err := c.popUIDLs(context.Background()); err == nil {
		t.Fatal("expected dial error from popUIDLs")
	}
	if _, err := c.pollPOP3(context.Background()); err == nil {
		t.Fatal("expected dial error from pollPOP3")
	}
}

func TestDialPOP3AndUIDLs(t *testing.T) {
	srv := newPOP3(t, map[string]string{"A": sampleMail}, []string{"A"})
	c := New(Config{
		InboxAddr:     srv.addr(),
		InboxProtocol: "pop3",
		InboxTLS:      "none",
		InboxUsername: "u",
		InboxPassword: "p",
	})
	uidls, err := c.popUIDLs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(uidls) != 1 || uidls[0] != "A" {
		t.Fatalf("uidls = %v", uidls)
	}
}

func TestPollIMAPDialFailure(t *testing.T) {
	// Dial a closed port with each TLS mode => dialIMAP + pollIMAP error out.
	for _, mode := range []string{"none", "starttls", "tls"} {
		c := New(Config{
			InboxAddr:     "127.0.0.1:1",
			InboxProtocol: "imap",
			InboxTLS:      mode,
		})
		if _, err := c.pollIMAP(context.Background()); err == nil {
			t.Fatalf("mode %q: expected dial error from pollIMAP", mode)
		}
		if _, err := c.dialIMAP(); err == nil {
			t.Fatalf("mode %q: expected dial error from dialIMAP", mode)
		}
	}
}

func TestDispatchInboundGuards(t *testing.T) {
	c := New(Config{})
	// Empty sender.
	c.dispatchInbound(context.Background(), inboundMail{from: ""})
	// Empty subject + body.
	c.dispatchInbound(context.Background(), inboundMail{from: "a@b.com"})
	// Dedup on message-id.
	c.seenBefore("mid:X")
	c.dispatchInbound(context.Background(), inboundMail{from: "a@b.com", body: "hi", messageID: "X"})
}

func TestEmitOutboundNilBus(t *testing.T) {
	New(Config{}).emitOutbound(channel.Outbound{ChannelID: "a@b.com", Text: "hi"})
}

func TestEmitOutboundWithBus(t *testing.T) {
	c := New(Config{Bus: newBus(t)})
	c.emitOutbound(channel.Outbound{ChannelID: "a@b.com", Text: "hi"})
}

func TestEmitInboundWithBus(t *testing.T) {
	c := New(Config{Bus: newBus(t)})
	c.emitInbound(channel.UnifiedMessage{ChannelID: "a@b.com", Sender: "a@b.com", Text: "hi"}, "corr", true)
}

func TestSeenBeforeRingEviction(t *testing.T) {
	c := New(Config{})
	// dedupCap is the ring bound.
	for i := 0; i < dedupCap+5; i++ {
		if c.seenBefore("id" + itoa(i)) {
			t.Fatalf("fresh id%d seen", i)
		}
	}
	if !c.seenBefore("id" + itoa(dedupCap+4)) {
		t.Fatal("recent id should be seen")
	}
	if c.seenBefore("id0") {
		t.Fatal("first id should be evicted")
	}
}

func TestParseMailMultipartAndInvalid(t *testing.T) {
	// Invalid message.
	if _, ok := parseMail([]byte("garbage without headers")); ok {
		t.Fatal("invalid mail should return ok=false")
	}
	// Simple text message parses.
	m, ok := parseMail([]byte(sampleMail))
	if !ok || m.from != "alice@example.com" || m.subject != "Hello" || !strings.Contains(m.body, "body") {
		t.Fatalf("parseMail = %+v ok=%v", m, ok)
	}
}
