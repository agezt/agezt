// SPDX-License-Identifier: MIT

package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
)

// fakeCP is an in-process stand-in for a running Agezt daemon's control plane.
//
// Why this exists: every Client method funnels through controlplane.Client,
// which speaks a line-delimited JSON protocol over TCP — one JSON Request
// (terminated by '\n') in, one-or-more JSON Response lines out. controlplane.
// Client is a concrete struct with unexported fields, so it can't be mocked at
// the Go type level; the only seam is the wire. So we stand up a real TCP
// listener that talks that exact protocol, write its addr+token into the
// runtime files Dial reads, and drive the whole client end to end. This proves
// the request encoding AND the response decoding, which a pure-parse test can't.
type fakeCP struct {
	ln      net.Listener
	baseDir string

	mu       sync.Mutex
	requests []controlplane.Request // every request the server received

	// handler returns the response lines (already JSON-serializable Response
	// values) for a given request. If it returns keepOpen, the server does not
	// close the connection after writing — used to exercise StreamUntilCancel.
	handler func(req controlplane.Request) (lines []controlplane.Response, keepOpen bool)
}

// startFakeCP boots the fake server, writes the runtime addr/token files under
// a temp baseDir, and returns the harness. The listener and any accept loop are
// torn down via t.Cleanup.
func startFakeCP(t *testing.T, handler func(controlplane.Request) ([]controlplane.Response, bool)) *fakeCP {
	t.Helper()

	// Make sure no stray AGEZT_TOKEN in the environment shadows the on-disk
	// token — NewClient prefers the env var when set, which would break the
	// clean "read token from disk" path we want to exercise.
	t.Setenv(brand.EnvPrefix+"TOKEN", "")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	base := t.TempDir()
	runtimeDir := filepath.Join(base, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	// control.addr / control.token are the file names controlplane.NewClient
	// reads. Writing them here is exactly what a live daemon does on startup.
	if err := os.WriteFile(filepath.Join(runtimeDir, "control.addr"), []byte(ln.Addr().String()), 0o600); err != nil {
		t.Fatalf("write addr: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "control.token"), []byte("test-token"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	cp := &fakeCP{ln: ln, baseDir: base, handler: handler}
	go cp.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return cp
}

func (f *fakeCP) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return // listener closed → shut the loop down
		}
		go f.handleConn(conn)
	}
}

func (f *fakeCP) handleConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}
	var req controlplane.Request
	if err := json.Unmarshal(line, &req); err != nil {
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()

	lines, keepOpen := f.handler(req)
	for _, resp := range lines {
		enc, _ := json.Marshal(resp)
		enc = append(enc, '\n')
		if _, werr := conn.Write(enc); werr != nil {
			return
		}
	}
	if keepOpen {
		// Hold the connection until the client closes it (StreamUntilCancel
		// closes the conn when its ctx is cancelled). Block on a read that
		// unblocks with EOF the moment the client hangs up.
		_, _ = reader.ReadBytes('\n')
	}
}

// lastRequest returns the most recent request the server saw, for asserting the
// client encoded the right command + args.
func (f *fakeCP) lastRequest(t *testing.T) controlplane.Request {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("fakeCP received no requests")
	}
	return f.requests[len(f.requests)-1]
}

// result is a convenience to build a single RespResult line.
func resultLine(m map[string]any) []controlplane.Response {
	return []controlplane.Response{{Type: controlplane.RespResult, Result: m}}
}

// dialFake connects a real sdk.Client at the fake server's baseDir.
func dialFake(t *testing.T, f *fakeCP) *Client {
	t.Helper()
	c, err := Dial(f.baseDir)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	return c
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

// --- Dial / DefaultBaseDir ------------------------------------------------

func TestDial_EmptyBaseDirResolvesDefault(t *testing.T) {
	// Point the default resolver at a temp dir that has NO runtime files, so
	// Dial("") resolves the default base (via paths.BaseDir → AGEZT_HOME) and
	// then fails at NewClient because no daemon is recorded. This drives the
	// baseDir == "" branch and the NewClient-error branch together.
	home := t.TempDir()
	t.Setenv(brand.EnvPrefix+"HOME", home)

	_, err := Dial("")
	if err == nil {
		t.Fatal("Dial(\"\") should error when the default base has no daemon recorded")
	}
}

func TestDial_DefaultBaseDirError(t *testing.T) {
	// Force paths.BaseDir to fail (no AGEZT_HOME, no home dir) so Dial's
	// "resolve default base" error branch runs.
	t.Setenv(brand.EnvPrefix+"HOME", "")
	// Blank every OS home var; if the host still resolves a home we skip.
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	_, err := Dial("")
	if err == nil {
		t.Skip("host still resolves a home dir; cannot force the BaseDir error branch")
	}
}

func TestDefaultBaseDir(t *testing.T) {
	want := filepath.Join(t.TempDir(), "explicit-home")
	t.Setenv(brand.EnvPrefix+"HOME", want)
	got, err := DefaultBaseDir()
	if err != nil {
		t.Fatalf("DefaultBaseDir: %v", err)
	}
	if got != want {
		t.Errorf("DefaultBaseDir = %q, want %q", got, want)
	}
}

// --- Runs -----------------------------------------------------------------

func TestRuns_SendsLimitAndParses(t *testing.T) {
	f := startFakeCP(t, func(req controlplane.Request) ([]controlplane.Response, bool) {
		return resultLine(map[string]any{"runs": []any{
			map[string]any{"correlation_id": "c1", "intent": "do", "status": "completed"},
		}}), false
	})
	c := dialFake(t, f)

	runs, err := c.Runs(ctx(t), 7)
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) != 1 || runs[0].CorrelationID != "c1" {
		t.Errorf("Runs returned %+v", runs)
	}
	req := f.lastRequest(t)
	if req.Cmd != controlplane.CmdRunsList {
		t.Errorf("cmd = %q, want %q", req.Cmd, controlplane.CmdRunsList)
	}
	if req.Args["limit"] != float64(7) {
		t.Errorf("limit arg = %v (%T), want 7", req.Args["limit"], req.Args["limit"])
	}
}

func TestRuns_NoLimitOmitsArg(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return resultLine(map[string]any{"runs": []any{}}), false
	})
	c := dialFake(t, f)

	if _, err := c.Runs(ctx(t), 0); err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if _, ok := f.lastRequest(t).Args["limit"]; ok {
		t.Error("limit <= 0 must omit the limit arg entirely")
	}
}

func TestRuns_ServerError(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return []controlplane.Response{{Type: controlplane.RespError, Error: "boom"}}, false
	})
	c := dialFake(t, f)
	if _, err := c.Runs(ctx(t), 1); err == nil {
		t.Fatal("Runs should surface a server error")
	}
}

// --- Approvals ------------------------------------------------------------

func TestPendingApprovals_Parses(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return resultLine(map[string]any{"pending": []any{
			map[string]any{
				"id": "a1", "capability": "shell.exec", "tool_name": "shell",
				"reason": "gated", "actor": "run-1",
				"input": map[string]any{"cmd": "ls"}, "timeout_unix": float64(1_700_000_000),
			},
		}}), false
	})
	c := dialFake(t, f)

	ps, err := c.PendingApprovals(ctx(t))
	if err != nil {
		t.Fatalf("PendingApprovals: %v", err)
	}
	if len(ps) != 1 {
		t.Fatalf("want 1 approval, got %d", len(ps))
	}
	a := ps[0]
	if a.ID != "a1" || a.Capability != "shell.exec" || a.Tool != "shell" || a.Actor != "run-1" {
		t.Errorf("approval fields wrong: %+v", a)
	}
	if a.Input == "" {
		t.Error("structured input should render as JSON, got empty")
	}
	if a.Timeout.IsZero() {
		t.Error("timeout_unix should decode to a non-zero time")
	}
	if f.lastRequest(t).Cmd != controlplane.CmdApprovals {
		t.Errorf("cmd = %q", f.lastRequest(t).Cmd)
	}
}

func TestPendingApprovals_Error(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return []controlplane.Response{{Type: controlplane.RespError, Error: "nope"}}, false
	})
	c := dialFake(t, f)
	if _, err := c.PendingApprovals(ctx(t)); err == nil {
		t.Fatal("PendingApprovals should surface a server error")
	}
}

func TestApproveAndDeny_SendDecideArgs(t *testing.T) {
	var seen []controlplane.Request
	f := startFakeCP(t, func(req controlplane.Request) ([]controlplane.Response, bool) {
		seen = append(seen, req)
		return resultLine(map[string]any{"ok": true}), false
	})
	c := dialFake(t, f)

	if err := c.Approve(ctx(t), "a1", "looks fine"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := c.Deny(ctx(t), "a2", "nope"); err != nil {
		t.Fatalf("Deny: %v", err)
	}

	f.mu.Lock()
	reqs := append([]controlplane.Request(nil), f.requests...)
	f.mu.Unlock()
	if len(reqs) != 2 {
		t.Fatalf("want 2 decide calls, got %d", len(reqs))
	}
	if reqs[0].Cmd != controlplane.CmdDecide || reqs[0].Args["decision"] != "grant" || reqs[0].Args["id"] != "a1" {
		t.Errorf("Approve sent wrong decide args: %+v", reqs[0].Args)
	}
	if reqs[1].Args["decision"] != "deny" || reqs[1].Args["reason"] != "nope" {
		t.Errorf("Deny sent wrong decide args: %+v", reqs[1].Args)
	}
}

func TestDecide_Error(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return []controlplane.Response{{Type: controlplane.RespError, Error: "denied"}}, false
	})
	c := dialFake(t, f)
	if err := c.Approve(ctx(t), "x", ""); err == nil {
		t.Fatal("Approve should surface a server error")
	}
}

// --- Mailbox --------------------------------------------------------------

func TestSendMail_And_Broadcast(t *testing.T) {
	f := startFakeCP(t, func(req controlplane.Request) ([]controlplane.Response, bool) {
		return resultLine(map[string]any{"sent": map[string]any{
			"id": "m-1", "from": req.Args["from"], "to": req.Args["to"], "text": req.Args["text"],
		}}), false
	})
	c := dialFake(t, f)

	m, err := c.SendMail(ctx(t), MailDraft{From: "app", To: "planner", Text: "hi"})
	if err != nil {
		t.Fatalf("SendMail: %v", err)
	}
	if m.ID != "m-1" || m.To != "planner" || m.Text != "hi" {
		t.Errorf("SendMail returned %+v", m)
	}
	if f.lastRequest(t).Cmd != controlplane.CmdBoardSend {
		t.Errorf("cmd = %q", f.lastRequest(t).Cmd)
	}

	bm, err := c.Broadcast(ctx(t), "app", "announce")
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if bm.To != "*" || bm.Text != "announce" {
		t.Errorf("Broadcast should address '*': %+v", bm)
	}
}

func TestSendMail_Error(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return []controlplane.Response{{Type: controlplane.RespError, Error: "board down"}}, false
	})
	c := dialFake(t, f)
	if _, err := c.SendMail(ctx(t), MailDraft{From: "a", To: "b", Text: "x"}); err == nil {
		t.Fatal("SendMail should surface a server error")
	}
}

func TestInbox_SendsArgsAndParses(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return resultLine(map[string]any{"waiting": []any{
			map[string]any{"id": "m-1", "to": "me", "text": "waiting"},
		}}), false
	})
	c := dialFake(t, f)

	mails, err := c.Inbox(ctx(t), "me", true, 5)
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(mails) != 1 || mails[0].ID != "m-1" {
		t.Errorf("Inbox returned %+v", mails)
	}
	req := f.lastRequest(t)
	if req.Cmd != controlplane.CmdBoardInbox || req.Args["to"] != "me" || req.Args["all"] != true || req.Args["limit"] != float64(5) {
		t.Errorf("Inbox sent wrong args: %+v", req.Args)
	}
}

func TestInbox_Error(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return []controlplane.Response{{Type: controlplane.RespError, Error: "x"}}, false
	})
	c := dialFake(t, f)
	if _, err := c.Inbox(ctx(t), "me", false, 0); err == nil {
		t.Fatal("Inbox should surface a server error")
	}
}

func TestAckMail(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return resultLine(map[string]any{"ok": true}), false
	})
	c := dialFake(t, f)
	if err := c.AckMail(ctx(t), "m-1", "me"); err != nil {
		t.Fatalf("AckMail: %v", err)
	}
	req := f.lastRequest(t)
	if req.Cmd != controlplane.CmdBoardAck || req.Args["id"] != "m-1" || req.Args["by"] != "me" {
		t.Errorf("AckMail sent wrong args: %+v", req.Args)
	}
}

func TestMailReplies(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return resultLine(map[string]any{"replies": []any{
			map[string]any{"id": "r-1", "reply_to": "m-1", "text": "answer"},
		}}), false
	})
	c := dialFake(t, f)

	rs, err := c.MailReplies(ctx(t), "m-1", 3)
	if err != nil {
		t.Fatalf("MailReplies: %v", err)
	}
	if len(rs) != 1 || rs[0].ReplyTo != "m-1" {
		t.Errorf("MailReplies returned %+v", rs)
	}
	req := f.lastRequest(t)
	if req.Cmd != controlplane.CmdBoardReplies || req.Args["id"] != "m-1" || req.Args["limit"] != float64(3) {
		t.Errorf("MailReplies sent wrong args: %+v", req.Args)
	}
}

func TestMailReplies_Error(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return []controlplane.Response{{Type: controlplane.RespError, Error: "x"}}, false
	})
	c := dialFake(t, f)
	if _, err := c.MailReplies(ctx(t), "m-1", 0); err == nil {
		t.Fatal("MailReplies should surface a server error")
	}
}

func TestMailMessages(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return resultLine(map[string]any{"messages": []any{
			map[string]any{"id": "m-1", "topic": "dm", "text": "hi"},
		}}), false
	})
	c := dialFake(t, f)

	msgs, err := c.MailMessages(ctx(t), "dm", 4)
	if err != nil {
		t.Fatalf("MailMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Topic != "dm" {
		t.Errorf("MailMessages returned %+v", msgs)
	}
	req := f.lastRequest(t)
	if req.Cmd != controlplane.CmdBoardRead || req.Args["topic"] != "dm" || req.Args["limit"] != float64(4) {
		t.Errorf("MailMessages sent wrong args: %+v", req.Args)
	}
}

func TestMailMessages_EmptyTopicOmitsArg(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return resultLine(map[string]any{"messages": []any{}}), false
	})
	c := dialFake(t, f)
	if _, err := c.MailMessages(ctx(t), "", 0); err != nil {
		t.Fatalf("MailMessages: %v", err)
	}
	if _, ok := f.lastRequest(t).Args["topic"]; ok {
		t.Error("empty topic must omit the topic arg")
	}
}

func TestMailMessages_Error(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return []controlplane.Response{{Type: controlplane.RespError, Error: "x"}}, false
	})
	c := dialFake(t, f)
	if _, err := c.MailMessages(ctx(t), "", 0); err == nil {
		t.Fatal("MailMessages should surface a server error")
	}
}

// --- WatchMail (StreamUntilCancel) ---------------------------------------

func TestWatchMail_DeliversPostedMails(t *testing.T) {
	// WatchMail subscribes to board.posted events and calls fn for each one it
	// can decode. We stream two board.posted events (one addressed to "me", one
	// a broadcast from someone else), then hold the conn open until the client's
	// ctx cancels — that's how the real pulse subscription behaves.
	payload := func(from, to, text string, help bool) json.RawMessage {
		b, _ := json.Marshal(map[string]any{
			"id": "m-x", "from": from, "to": to, "text": text, "help": help,
		})
		return b
	}
	f := startFakeCP(t, func(req controlplane.Request) ([]controlplane.Response, bool) {
		// The CmdBoardGet metadata fetch (fired per delivered event) gets a
		// proper {message:{...}} result so WatchMail's success branch — which
		// replaces the event-derived Mail with the fuller board view — runs.
		if req.Cmd == controlplane.CmdBoardGet {
			return resultLine(map[string]any{"message": map[string]any{
				"id": req.Args["id"], "from": "planner", "to": "me",
				"text": "full view", "topic": "dm",
			}}), false
		}
		ev1 := &event.Event{Kind: event.KindBoardPosted, Payload: payload("planner", "me", "for you", false)}
		ev2 := &event.Event{Kind: event.KindBoardPosted, Payload: payload("worker", "*", "to all", true)}
		// A non-board event and a payload-less event exercise WatchMail's
		// early-return guards (wrong kind / empty payload).
		evSkip := &event.Event{Kind: event.KindLLMToken, Payload: json.RawMessage(`{"text":"x"}`)}
		evEmpty := &event.Event{Kind: event.KindBoardPosted}
		// A valid board post directed at someone ELSE — passes the payload
		// guards but fails mailForName("me", ...), exercising that return.
		evOther := &event.Event{Kind: event.KindBoardPosted, Payload: payload("planner", "someone-else", "private", false)}
		// A board post whose payload is valid but non-object shape can't
		// unmarshal into the struct — exercises the Unmarshal-error return.
		evBadPayload := &event.Event{Kind: event.KindBoardPosted, Payload: json.RawMessage(`"not-an-object"`)}
		return []controlplane.Response{
			{Type: controlplane.RespEvent, Event: evSkip},
			{Type: controlplane.RespEvent, Event: evEmpty},
			{Type: controlplane.RespEvent, Event: evOther},
			{Type: controlplane.RespEvent, Event: evBadPayload},
			{Type: controlplane.RespEvent, Event: ev1},
			{Type: controlplane.RespEvent, Event: ev2},
		}, true // keepOpen: mimic an open pulse stream
	})
	c := dialFake(t, f)

	watchCtx, cancel := context.WithCancel(context.Background())
	var (
		mu   sync.Mutex
		got  []Mail
		done = make(chan error, 1)
	)
	go func() {
		done <- c.WatchMail(watchCtx, "me", func(m Mail) {
			mu.Lock()
			got = append(got, m)
			mu.Unlock()
		})
	}()

	// Two events pass the name-filter for "me": the direct message (ev1) and
	// the foreign broadcast (ev2). Wait for BOTH before cancelling so the whole
	// per-event path — including the board_get success branch — runs
	// deterministically on every run rather than racing the cancel.
	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatalf("WatchMail delivered %d/2 mails before the deadline", n)
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()

	select {
	case err := <-done:
		// ctx-cancel is the documented clean-shutdown path → nil error.
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("WatchMail returned %v, want nil/Canceled on ctx cancel", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WatchMail did not return after ctx cancel")
	}

	// WatchMail first opens a pulse subscription, then (per delivered event)
	// fetches the full message view via CmdBoardGet. So we assert the
	// subscription request exists among ALL requests rather than checking the
	// last one, which is a board_get metadata fetch.
	f.mu.Lock()
	reqs := append([]controlplane.Request(nil), f.requests...)
	f.mu.Unlock()
	var sub *controlplane.Request
	for i := range reqs {
		if reqs[i].Cmd == controlplane.CmdPulseSubscribe {
			sub = &reqs[i]
			break
		}
	}
	if sub == nil {
		t.Fatalf("no %q subscription request seen; got %d requests", controlplane.CmdPulseSubscribe, len(reqs))
	}
	if sub.Args["pattern"] != "board.>" {
		t.Errorf("pattern arg = %v, want board.>", sub.Args["pattern"])
	}
}

// TestWatchMail_BoardGetErrorFallsBackToEvent covers WatchMail's fallback path:
// when the per-event CmdBoardGet metadata fetch fails, it keeps the Mail it
// already built from the event payload instead of the fuller board view. We
// make board_get return a server error and assert the delivered Mail still
// carries the event-derived fields.
func TestWatchMail_BoardGetErrorFallsBackToEvent(t *testing.T) {
	post, _ := json.Marshal(map[string]any{
		"id": "ev-1", "from": "planner", "to": "me", "text": "from-event", "help": false,
	})
	f := startFakeCP(t, func(req controlplane.Request) ([]controlplane.Response, bool) {
		if req.Cmd == controlplane.CmdBoardGet {
			// Fail the metadata fetch so WatchMail keeps the event-derived Mail.
			return []controlplane.Response{{Type: controlplane.RespError, Error: "evicted"}}, false
		}
		ev := &event.Event{Kind: event.KindBoardPosted, Payload: post}
		return []controlplane.Response{{Type: controlplane.RespEvent, Event: ev}}, true
	})
	c := dialFake(t, f)

	watchCtx, cancel := context.WithCancel(context.Background())
	var (
		mu  sync.Mutex
		got []Mail
	)
	go func() {
		_ = c.WatchMail(watchCtx, "me", func(m Mail) {
			mu.Lock()
			got = append(got, m)
			mu.Unlock()
		})
	}()

	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 1 {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("WatchMail delivered no mail before the deadline")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()

	mu.Lock()
	m := got[0]
	mu.Unlock()
	// The event-derived Mail (not a board_get view) must survive the fetch error.
	if m.ID != "ev-1" || m.From != "planner" || m.To != "me" {
		t.Errorf("board_get failure should keep the event-derived Mail, got %+v", m)
	}
}

// --- Run / RunStream ------------------------------------------------------

func TestRun_ParsesFinalResult(t *testing.T) {
	f := startFakeCP(t, func(req controlplane.Request) ([]controlplane.Response, bool) {
		return resultLine(map[string]any{
			"answer": "42", "correlation_id": "run-1", "model": "m1",
			"iters": float64(3), "spent_mc": float64(2.5e8),
		}), false
	})
	c := dialFake(t, f)

	res, err := c.Run(ctx(t), "the question", WithModel("m1"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "42" || res.CorrelationID != "run-1" || res.Model != "m1" {
		t.Errorf("Run result wrong: %+v", res)
	}
	req := f.lastRequest(t)
	if req.Cmd != controlplane.CmdRun || req.Args["intent"] != "the question" || req.Args["model"] != "m1" {
		t.Errorf("Run sent wrong args: %+v", req.Args)
	}
}

func TestRunStream_InvokesCallbackPerEvent(t *testing.T) {
	tokenPayload, _ := json.Marshal(map[string]any{"text": "hel"})
	f := startFakeCP(t, func(req controlplane.Request) ([]controlplane.Response, bool) {
		ev := &event.Event{Kind: event.KindLLMToken, Payload: tokenPayload}
		return []controlplane.Response{
			{Type: controlplane.RespEvent, Event: ev},
			{Type: controlplane.RespResult, Result: map[string]any{"answer": "hello", "correlation_id": "r-2"}},
		}, false
	})
	c := dialFake(t, f)

	var streamed []string
	res, err := c.RunStream(ctx(t), "go", func(ev *Event) {
		if txt, ok := TokenText(ev); ok {
			streamed = append(streamed, txt)
		}
	})
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	if res.Answer != "hello" || res.CorrelationID != "r-2" {
		t.Errorf("RunStream final result wrong: %+v", res)
	}
	if len(streamed) != 1 || streamed[0] != "hel" {
		t.Errorf("onEvent should have seen the token delta, got %v", streamed)
	}
}

func TestRunStream_NilCallback(t *testing.T) {
	// Run() passes a nil onEvent; RunStream must tolerate that (its internal cb
	// guards nil) and still return the final result.
	f := startFakeCP(t, func(req controlplane.Request) ([]controlplane.Response, bool) {
		ev := &event.Event{Kind: event.KindLLMToken}
		return []controlplane.Response{
			{Type: controlplane.RespEvent, Event: ev},
			{Type: controlplane.RespResult, Result: map[string]any{"answer": "ok"}},
		}, false
	})
	c := dialFake(t, f)

	res, err := c.RunStream(ctx(t), "go", nil)
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	if res.Answer != "ok" {
		t.Errorf("RunStream answer = %q", res.Answer)
	}
}

func TestRunStream_ServerError(t *testing.T) {
	f := startFakeCP(t, func(controlplane.Request) ([]controlplane.Response, bool) {
		return []controlplane.Response{{Type: controlplane.RespError, Error: "run failed"}}, false
	})
	c := dialFake(t, f)
	if _, err := c.Run(ctx(t), "go"); err == nil {
		t.Fatal("Run should surface a server error")
	}
}
