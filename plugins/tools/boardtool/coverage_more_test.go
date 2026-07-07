// SPDX-License-Identifier: MIT

package boardtool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/board"
)

func TestBoardCoverageNewBindBindStoreOnPostCurrent(t *testing.T) {
	tool := New()
	if tool.store != nil {
		t.Fatal("New should leave store nil")
	}
	if tool.now == nil {
		t.Fatal("New should wire default now()")
	}
	if tool.notify != nil {
		t.Fatal("New should leave notify nil")
	}

	// Bind into a fresh temp dir.
	if err := tool.Bind(t.TempDir()); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if tool.store == nil {
		t.Fatal("Bind should set store")
	}
	if err := tool.Bind(t.TempDir()); err != nil {
		t.Fatalf("Bind again: %v", err)
	}

	// Bind error path: empty path.
	if err := tool.Bind(""); err == nil {
		t.Fatal("empty dir should error")
	}

	// Re-create so we can test bindStore (the internal injection helper used
	// by the existing tests; BindStore is the public *board.Store path).
	tool = New()
	store := &fakeStore{}
	tool.bindStore(store)
	if tool.store == nil {
		t.Fatal("bindStore should set store")
	}
	// bindStore overwrites unconditionally (no nil guard) — document that.
	tool.bindStore(nil)
	if tool.store != nil {
		t.Fatal("bindStore(nil) currently overwrites with nil; behavior is intentional or a future guard")
	}
	// Re-set for downstream tests.
	tool.bindStore(store)

	// OnPost: notifier wired.
	notified := 0
	tool.OnPost(func(board.Message, string) { notified++ })
	if tool.notify == nil {
		t.Fatal("OnPost should wire notify")
	}

	// current() with default now().
	st, now, not := tool.current()
	if st == nil || now == nil || not == nil {
		t.Fatal("current() should return all three")
	}

	// current() with now==nil falls back to time.Now.
	tool.now = nil
	_, now, _ = tool.current()
	if now == nil {
		t.Fatal("current() should fall back to time.Now")
	}
}

func TestBoardCoverageClampLimit(t *testing.T) {
	cases := map[int]int{
		0:   DefaultReadLimit,
		-7:  DefaultReadLimit,
		5:   5,
		99:  99,
		100: 100,
		500: MaxReadLimit,
	}
	for in, want := range cases {
		if got := clampLimit(in); got != want {
			t.Fatalf("clampLimit(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestBoardCoverageInvokeValidationAndOpInferral(t *testing.T) {
	tool := New()
	tool.bindStore(&fakeStore{})

	// Parse error.
	_, err := tool.Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	// Empty op + no text → defaults to read (returns empty list, no error).
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"topic":"general"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("read default = %+v", res)
	}
}

func TestBoardCoverageInvokeInferredOps(t *testing.T) {
	store := &fakeStore{}
	tool := New()
	tool.bindStore(store)

	// text + to → inferred "send".
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"to":"alice","text":"hi","from":"bob"}`))
	if err != nil || res.IsError {
		t.Fatalf("send: %+v err %v", res, err)
	}
	if !strings.Contains(res.Output, `"to": "alice"`) {
		t.Fatalf("send output missing to: %s", res.Output)
	}

	// Pre-seed a message so the reply branch can resolve Get.
	store.msgs = append(store.msgs, board.Message{ID: "msg-1", From: "asker", Topic: "help"})
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"id":"msg-1","text":"answer","from":"replier"}`))
	if err != nil {
		t.Fatalf("Invoke reply: %v", err)
	}
	if res.IsError {
		t.Fatalf("reply should succeed when Get returns the message: %+v", res)
	}
	if !strings.Contains(res.Output, `"replied"`) {
		t.Fatalf("reply output missing replied key: %s", res.Output)
	}
	if !strings.Contains(res.Output, `"reply_to": "msg-1"`) {
		t.Fatalf("reply output missing reply_to: %s", res.Output)
	}

	// text only → inferred "post".
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"topic":"general","text":"hello","from":"me"}`))
	if err != nil || res.IsError {
		t.Fatalf("post: %+v err %v", res, err)
	}
	if !strings.Contains(res.Output, `"posted"`) {
		t.Fatalf("post output missing posted key: %s", res.Output)
	}
}
