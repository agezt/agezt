// SPDX-License-Identifier: MIT

package irc

import (
	"bufio"
	"context"
	"net"
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

// ircServer is a minimal fake ircd used to drive the client's session loop.
type ircServer struct {
	ln       net.Listener
	mu       sync.Mutex
	received []string
	lineCh   chan string
}

func newIRCServer(t *testing.T) *ircServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &ircServer{ln: ln, lineCh: make(chan string, 64)}
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *ircServer) addr() string { return s.ln.Addr().String() }

func (s *ircServer) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.received...)
}

// serveOne accepts one client, records inbound lines, and lets a script send
// server->client lines through the returned conn writer.
func (s *ircServer) serveOne(t *testing.T, script func(conn net.Conn)) {
	go func() {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		go func() {
			r := bufio.NewReader(conn)
			for {
				line, err := r.ReadString('\n')
				if err != nil {
					return
				}
				trimmed := strings.TrimRight(line, "\r\n")
				s.mu.Lock()
				s.received = append(s.received, trimmed)
				s.mu.Unlock()
				select {
				case s.lineCh <- trimmed:
				default:
				}
			}
		}()
		if script != nil {
			script(conn)
		}
		// Keep the connection open a little so the client can finish.
		time.Sleep(300 * time.Millisecond)
	}()
}

// waitFor blocks until a received line matching pred appears or timeout.
func (s *ircServer) waitFor(pred func(string) bool, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		for _, l := range s.lines() {
			if pred(l) {
				return true
			}
		}
		select {
		case <-s.lineCh:
		case <-deadline:
			return false
		}
	}
}

func TestSessionRegistersJoinsAndReplies(t *testing.T) {
	srv := newIRCServer(t)
	var replied bool
	handlerDone := make(chan struct{}, 1)

	srv.serveOne(t, func(conn net.Conn) {
		// Wait for the client to send NICK/USER, then welcome + drive traffic.
		_, _ = conn.Write([]byte(":srv 001 bot :Welcome\r\n"))
		_, _ = conn.Write([]byte("PING :srv123\r\n"))
		// A channel PRIVMSG from an allowlisted room.
		_, _ = conn.Write([]byte(":alice!a@h PRIVMSG #room :hello bot\r\n"))
	})

	ch := New(Config{
		Server:    srv.addr(),
		Nick:      "bot",
		Password:  "sekret",
		Channels:  []string{"#room"},
		Allowlist: channel.NewAllowlist([]string{"#room"}),
		Bus:       newBus(t),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			if m.Text == "hello bot" && m.ChannelID == "#room" && m.Sender == "alice" {
				replied = true
			}
			select {
			case handlerDone <- struct{}{}:
			default:
			}
			return channel.Reply{Text: "hi alice"}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = ch.Start(ctx) }()

	// Registration lines.
	if !srv.waitFor(func(l string) bool { return l == "PASS sekret" }, 2*time.Second) {
		t.Fatal("PASS not sent")
	}
	if !srv.waitFor(func(l string) bool { return l == "NICK bot" }, 2*time.Second) {
		t.Fatal("NICK not sent")
	}
	if !srv.waitFor(func(l string) bool { return strings.HasPrefix(l, "USER bot ") }, 2*time.Second) {
		t.Fatal("USER not sent")
	}
	// JOIN after welcome.
	if !srv.waitFor(func(l string) bool { return l == "JOIN #room" }, 2*time.Second) {
		t.Fatal("JOIN not sent")
	}
	// PONG after PING.
	if !srv.waitFor(func(l string) bool { return l == "PONG :srv123" }, 2*time.Second) {
		t.Fatal("PONG not sent")
	}
	// The agent reply is delivered as a PRIVMSG back to #room.
	if !srv.waitFor(func(l string) bool { return l == "PRIVMSG #room :hi alice" }, 2*time.Second) {
		t.Fatal("reply PRIVMSG not sent")
	}

	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not invoked")
	}
	if !replied {
		t.Fatal("handler did not observe expected message")
	}
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestSessionDMRepliesToSender(t *testing.T) {
	srv := newIRCServer(t)
	srv.serveOne(t, func(conn net.Conn) {
		_, _ = conn.Write([]byte(":srv 001 bot :Welcome\r\n"))
		// A direct message (target is our nick) => reply goes to the sender.
		_, _ = conn.Write([]byte(":carol!c@h PRIVMSG bot :ping\r\n"))
	})
	ch := New(Config{
		Server:    srv.addr(),
		Nick:      "bot",
		Allowlist: channel.NewAllowlist([]string{"carol"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{Text: "pong"}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ch.Start(ctx) }()
	if !srv.waitFor(func(l string) bool { return l == "PRIVMSG carol :pong" }, 2*time.Second) {
		t.Fatal("DM reply not routed to sender")
	}
}

func TestSessionHandlerErrorSendsApology(t *testing.T) {
	srv := newIRCServer(t)
	srv.serveOne(t, func(conn net.Conn) {
		_, _ = conn.Write([]byte(":srv 001 bot :Welcome\r\n"))
		_, _ = conn.Write([]byte(":dave!d@h PRIVMSG #room :boom\r\n"))
	})
	ch := New(Config{
		Server:    srv.addr(),
		Nick:      "bot",
		Channels:  []string{"#room"},
		Allowlist: channel.NewAllowlist([]string{"#room"}),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			return channel.Reply{}, context.DeadlineExceeded
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ch.Start(ctx) }()
	if !srv.waitFor(func(l string) bool { return strings.Contains(l, "sorry") }, 2*time.Second) {
		t.Fatal("apology not sent on handler error")
	}
}

func TestSessionNotAllowedNoHandler(t *testing.T) {
	srv := newIRCServer(t)
	var called bool
	srv.serveOne(t, func(conn net.Conn) {
		_, _ = conn.Write([]byte(":srv 001 bot :Welcome\r\n"))
		_, _ = conn.Write([]byte(":eve!e@h PRIVMSG #other :hi\r\n"))
	})
	ch := New(Config{
		Server:    srv.addr(),
		Nick:      "bot",
		Allowlist: channel.NewAllowlist([]string{"#room"}), // #other not allowed
		Bus:       newBus(t),
		Handler: func(ctx context.Context, m channel.UnifiedMessage, corr string) (channel.Reply, error) {
			called = true
			return channel.Reply{Text: "x"}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ch.Start(ctx) }()
	// Give it time to process the (rejected) message.
	time.Sleep(400 * time.Millisecond)
	if called {
		t.Fatal("handler must not run for non-allowlisted target")
	}
}

func TestStartReturnsWhenCtxCancelledBeforeDial(t *testing.T) {
	ch := New(Config{Server: "127.0.0.1:1", Nick: "bot"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	done := make(chan error, 1)
	go func() { done <- ch.Start(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return on pre-cancelled ctx")
	}
}

func TestStartReconnectsWithBackoff(t *testing.T) {
	// Dial a closed port so session() errors immediately; Start should retry
	// (bounded backoff) and then exit cleanly when ctx is cancelled.
	ch := New(Config{Server: "127.0.0.1:1", Nick: "bot"})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ch.Start(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after ctx timeout")
	}
}

func TestSendWritesWhenConnected(t *testing.T) {
	srv := newIRCServer(t)
	srv.serveOne(t, func(conn net.Conn) {
		_, _ = conn.Write([]byte(":srv 001 bot :Welcome\r\n"))
		time.Sleep(500 * time.Millisecond)
	})
	ch := New(Config{Server: srv.addr(), Nick: "bot", Bus: newBus(t)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ch.Start(ctx) }()
	// Wait until the client is registered (welcome triggers no JOIN here, but
	// the conn is set once session() starts writing).
	if !srv.waitFor(func(l string) bool { return l == "NICK bot" }, 2*time.Second) {
		t.Fatal("client did not register")
	}
	// Multi-line send exercises splitLines + writeLine + bus publish.
	if err := ch.Send(ctx, channel.Outbound{ChannelID: "#room", Text: "line1\nline2"}); err != nil {
		t.Fatalf("Send = %v", err)
	}
	if !srv.waitFor(func(l string) bool { return l == "PRIVMSG #room :line1" }, 2*time.Second) {
		t.Fatal("line1 not sent")
	}
	if !srv.waitFor(func(l string) bool { return l == "PRIVMSG #room :line2" }, 2*time.Second) {
		t.Fatal("line2 not sent")
	}
}

func TestHandlePrivmsgDropsEmpty(t *testing.T) {
	ch := New(Config{Nick: "bot"})
	// Malformed params (no space) => splitPrivmsg returns ok=false, no panic.
	ch.handlePrivmsg(context.Background(), "x!y@z", "nospace")
	// Empty message text => dropped.
	ch.handlePrivmsg(context.Background(), "x!y@z", "#room :")
}

func TestEmitInboundNilBus(t *testing.T) {
	New(Config{}).emitInbound(channel.UnifiedMessage{}, "corr", true)
}

func TestParseLinePrefixOnly(t *testing.T) {
	// A ":prefix" line with no space returns just the prefix.
	prefix, cmd, params := parseLine(":onlyprefix")
	if prefix != "onlyprefix" || cmd != "" || params != "" {
		t.Fatalf("parseLine = %q / %q / %q", prefix, cmd, params)
	}
	// A bare command with no params.
	_, cmd2, params2 := parseLine("QUIT")
	if cmd2 != "QUIT" || params2 != "" {
		t.Fatalf("bare cmd = %q / %q", cmd2, params2)
	}
}

func TestNickOfNoBang(t *testing.T) {
	if got := nickOf("serveronly"); got != "serveronly" {
		t.Fatalf("nickOf = %q", got)
	}
}

func TestWriteLineClampsAndNoConn(t *testing.T) {
	// No connection => writeLine is a no-op (early return), no panic.
	New(Config{}).writeLine("anything")
}
