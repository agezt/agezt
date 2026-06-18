// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"
	"testing"
	"time"
)

func TestToolMemo_ExpiredKeyDoesNotEvictFreshReplacement(t *testing.T) {
	now := time.Unix(100, 0)
	m := NewToolMemo(time.Second, 1)
	m.now = func() time.Time { return now }

	inputA := json.RawMessage(`{"x":1}`)
	inputB := json.RawMessage(`{"x":2}`)
	m.Set("read", inputA, Result{Output: "old-a"})

	now = now.Add(2 * time.Second)
	if _, ok := m.Get("read", inputA); ok {
		t.Fatal("expired memo entry unexpectedly returned")
	}

	m.Set("read", inputA, Result{Output: "new-a"})
	if got, ok := m.Get("read", inputA); !ok || got.Output != "new-a" {
		t.Fatalf("fresh replacement = {%q,%v}, want new-a,true", got.Output, ok)
	}

	m.Set("read", inputB, Result{Output: "b"})
	if got, ok := m.Get("read", inputB); !ok || got.Output != "b" {
		t.Fatalf("newest entry = {%q,%v}, want b,true", got.Output, ok)
	}
}

func TestToolMemo_DoesNotCacheErrorResults(t *testing.T) {
	m := NewToolMemo(time.Minute, 8)
	input := json.RawMessage(`{}`)
	m.Set("read", input, Result{Output: "boom", IsError: true})
	if _, ok := m.Get("read", input); ok {
		t.Fatal("error result was cached")
	}
}
