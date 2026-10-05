// SPDX-License-Identifier: MIT

package board_test

import (
	"context"
	"encoding/json"
	"errors"
	appboard "github.com/agezt/agezt/kernel/app/board"
	shared "github.com/agezt/agezt/kernel/board"
	"reflect"
	"sort"
	"strconv"
	"testing"
)

func boardFixture(t *testing.T) (*shared.Store, *appboard.Service) {
	t.Helper()
	store, err := shared.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store, appboard.New(store, nil)
}
func TestBoardReadRetainsCursorTieBreakTotalAndUnboundedZero(t *testing.T) {
	store, s := boardFixture(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := store.Post("topic", "writer", "owned-"+strconv.Itoa(i), 100); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Post("other", "writer", "other", 200); err != nil {
		t.Fatal(err)
	}
	expected := store.Read("topic", 0)
	sort.SliceStable(expected, func(i, j int) bool { return expected[i].ID > expected[j].ID })
	first, err := s.Read(ctx, appboard.ReadInput{Topic: "topic", Limit: 2})
	if err != nil || first.Count != 2 || first.Total != 5 || first.Topics["topic"] != 5 || first.Topics["other"] != 1 || first.Messages[0].ID != expected[0].ID || first.Messages[1].ID != expected[1].ID || first.NextCursor != "100:"+expected[1].ID {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := s.Read(ctx, appboard.ReadInput{Topic: "topic", Limit: 2, Cursor: first.NextCursor})
	if err != nil || second.Count != 2 || second.Total != 5 || second.Messages[0].ID != expected[2].ID || second.Messages[1].ID != expected[3].ID || second.NextCursor != "100:"+expected[3].ID {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	last, err := s.Read(ctx, appboard.ReadInput{Topic: "topic", Limit: 2, Cursor: second.NextCursor})
	if err != nil || last.Count != 1 || last.Total != 5 || last.Messages[0].ID != expected[4].ID || last.NextCursor != "" {
		t.Fatalf("last=%+v err=%v", last, err)
	}
	for _, cursor := range []string{"", "malformed", "999999999999999999999999:"} {
		out, err := s.Read(ctx, appboard.ReadInput{Topic: "topic", Limit: 0, Cursor: cursor})
		if err != nil || out.Count != 5 || out.NextCursor != "" {
			t.Fatalf("unbounded cursor=%q out=%+v err=%v", cursor, out, err)
		}
	}
	exact, err := s.Read(ctx, appboard.ReadInput{Topic: "topic", Limit: 5})
	if err != nil || exact.NextCursor != "" {
		t.Fatalf("exact full page=%+v err=%v", exact, err)
	}
}
func TestBoardServiceRetainsSendRoutingNotifierAndWire(t *testing.T) {
	store, _ := boardFixture(t)
	notifications := []shared.Message{}
	correlations := []string{}
	s := appboard.New(store, func(m shared.Message, corr string) {
		notifications = append(notifications, m)
		correlations = append(correlations, corr)
	})
	ctx := context.Background()
	post, err := s.Send(ctx, appboard.SendInput{Topic: "status", From: "writer", Text: "post", CorrelationID: "inbound"})
	if err != nil || post.Sent.Topic != "status" || post.CorrelationID != "inbound" || post.Sent.ID == "" || len(notifications) != 1 || correlations[0] != "inbound" {
		t.Fatalf("post=%+v notify=%v err=%v", post, correlations, err)
	}
	dm, err := s.Send(ctx, appboard.SendInput{From: "asker", To: "reviewer", Text: "question"})
	if err != nil || dm.Sent.Topic != "dm" || dm.Sent.To != "reviewer" {
		t.Fatalf("dm=%+v err=%v", dm, err)
	}
	reply, err := s.Send(ctx, appboard.SendInput{From: "reviewer", To: "ignored", Topic: "ignored", Text: "answer", ReplyTo: dm.Sent.ID, Help: true})
	if err != nil || reply.Sent.ReplyTo != dm.Sent.ID || reply.Sent.Topic != "dm" || reply.Sent.To != "asker" || reply.Sent.Help {
		t.Fatalf("reply precedence=%+v err=%v", reply, err)
	}
	broadcast, err := s.Send(ctx, appboard.SendInput{From: "writer", To: "*", Text: "everyone"})
	if err != nil || broadcast.Sent.Topic != "broadcast" || broadcast.Sent.To != "*" {
		t.Fatalf("broadcast=%+v err=%v", broadcast, err)
	}
	help, err := s.Send(ctx, appboard.SendInput{From: "asker", Text: "help", Help: true})
	if err != nil || help.Sent.Topic != "help" || help.Sent.To != "*" || !help.Sent.Help {
		t.Fatalf("help=%+v err=%v", help, err)
	}
	open, err := s.Help(ctx, appboard.LimitInput{Limit: 50})
	if err != nil || open.Count != 1 || open.OpenHelp[0].ID != help.Sent.ID {
		t.Fatalf("open help=%+v err=%v", open, err)
	}
	answers, err := s.Replies(ctx, appboard.RepliesInput{ID: dm.Sent.ID, Limit: 50})
	if err != nil || answers.Count != 1 || answers.Replies[0].ID != reply.Sent.ID {
		t.Fatalf("replies=%+v err=%v", answers, err)
	}
	raw, err := json.Marshal(post.Sent)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["ts_unix_ms"]; !ok {
		t.Fatalf("timestamp wire=%s", raw)
	}
	if _, ok := fields["ts_ms"]; ok {
		t.Fatalf("store timestamp leaked=%s", raw)
	}
	if _, ok := fields["help"]; ok {
		t.Fatalf("false help should be absent=%s", raw)
	}
	before := len(notifications)
	for _, in := range []appboard.SendInput{{}, {Text: "no address"}, {Text: "missing reply", ReplyTo: "absent"}} {
		if _, err := s.Send(ctx, in); err == nil {
			t.Fatalf("invalid send=%+v", in)
		}
	}
	if len(notifications) != before {
		t.Fatal("invalid send notified")
	}
}
func TestBoardServiceRetainsInboxAckEmptyWireAndOwnedViews(t *testing.T) {
	store, s := boardFixture(t)
	ctx := context.Background()
	empty, err := s.Read(ctx, appboard.ReadInput{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"messages":[],"topics":{},"count":0,"total":0}` {
		t.Fatalf("empty read=%s", raw)
	}
	message, err := store.Send(shared.Message{Topic: "dm", From: "writer", To: "Reviewer", Text: "owned"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := s.Inbox(ctx, appboard.InboxInput{To: "reviewer", Limit: 50})
	if err != nil || inbox.Count != 1 {
		t.Fatalf("inbox=%+v err=%v", inbox, err)
	}
	acked, err := s.Ack(ctx, appboard.AckInput{ID: message.ID, By: "Reviewer"})
	if err != nil || !acked.Acked || acked.ID != message.ID || acked.By != "Reviewer" {
		t.Fatalf("ack=%+v err=%v", acked, err)
	}
	inbox, err = s.Inbox(ctx, appboard.InboxInput{To: "reviewer", Limit: 50})
	if err != nil || inbox.Count != 0 || inbox.Waiting == nil {
		t.Fatalf("acked inbox=%+v err=%v", inbox, err)
	}
	all, err := s.Inbox(ctx, appboard.InboxInput{To: "reviewer", Limit: 50, All: true})
	if err != nil || all.Count != 1 {
		t.Fatalf("all inbox=%+v err=%v", all, err)
	}
	got, err := s.Get(ctx, appboard.GetInput{ID: message.ID})
	if err != nil || got.Message.TSUnixMS != 0 || !reflect.DeepEqual(got.Message.AckedBy, []string{"Reviewer"}) {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	got.Message.AckedBy[0] = "changed"
	stored, found := store.Get(message.ID)
	if !found || stored.AckedBy[0] != "Reviewer" {
		t.Fatal("output aliases store acknowledgement slice")
	}
	if _, err := s.Ack(ctx, appboard.AckInput{ID: message.ID, By: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	stored, _ = store.Get(message.ID)
	if len(stored.AckedBy) != 1 {
		t.Fatal("ack no longer idempotent")
	}
	for _, run := range []func() error{func() error { _, err := s.Get(ctx, appboard.GetInput{}); return err }, func() error { _, err := s.Get(ctx, appboard.GetInput{ID: "absent"}); return err }, func() error { _, err := s.Ack(ctx, appboard.AckInput{ID: "absent", By: "reviewer"}); return err }, func() error { _, err := s.Replies(ctx, appboard.RepliesInput{}); return err }, func() error { _, err := s.Inbox(ctx, appboard.InboxInput{}); return err }} {
		if err := run(); err == nil {
			t.Fatal("missing required identity silently admitted")
		}
	}
}

type failingBoard struct {
	*shared.Store
	cause error
}

func (s failingBoard) Post(string, string, string, int64) (shared.Message, error) {
	return shared.Message{}, s.cause
}
func (s failingBoard) Ack(string, string) (shared.Message, bool, error) {
	return shared.Message{}, false, s.cause
}
func TestBoardServiceReturnsWriteCauseBeforeNotifier(t *testing.T) {
	store, _ := boardFixture(t)
	cause := errors.New("owned board persistence failure")
	calls := 0
	s := appboard.New(failingBoard{Store: store, cause: cause}, func(shared.Message, string) { calls++ })
	if _, err := s.Send(context.Background(), appboard.SendInput{Topic: "topic", Text: "owned"}); err != cause {
		t.Fatalf("send cause=%v", err)
	}
	if _, err := s.Ack(context.Background(), appboard.AckInput{ID: "owned", By: "reader"}); err != cause {
		t.Fatalf("ack cause=%v", err)
	}
	if calls != 0 {
		t.Fatal("failed write notified")
	}
}
