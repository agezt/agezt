// SPDX-License-Identifier: MIT

package restapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/event"
)

// TestMailboxWatch_FiltersAndStreams drives the SSE watch end to end over a
// real HTTP server: a watcher on one name receives its DM and a foreign
// broadcast — with the full text resolved from the store — and nothing else.
func TestMailboxWatch_FiltersAndStreams(t *testing.T) {
	s := newServer(t, &fakeEngine{model: "m"}, "tok")
	st, err := board.Open(t.TempDir())
	if err != nil {
		t.Fatalf("board.Open: %v", err)
	}
	// Wire the notifier the way the daemon does: publish board.posted with the
	// metadata payload (no text) on the server's bus.
	s.SetMailbox(st, func(m board.Message, corr string) {
		_, _ = s.bus.Publish(event.Spec{
			Subject:       "board.test",
			Kind:          event.KindBoardPosted,
			Actor:         "board",
			CorrelationID: corr,
			Payload: map[string]any{
				"id": m.ID, "topic": m.Topic, "from": m.From, "to": m.To, "help": m.Help,
			},
		})
	})
	post := func(msg board.Message) {
		t.Helper()
		m, err := st.Send(msg, time.Now().UnixMilli())
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		s.boardNotify(m, "")
	}

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		ts.URL+"/api/v1/mailbox/watch?name=researcher", nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("watch request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("watch status: %d", resp.StatusCode)
	}

	// frames yields (event, data) pairs parsed from the SSE stream.
	scanner := bufio.NewScanner(resp.Body)
	nextFrame := func() (string, map[string]any) {
		t.Helper()
		eventName := ""
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				eventName = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				var data map[string]any
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data); err != nil {
					t.Fatalf("bad frame data: %v", err)
				}
				return eventName, data
			}
		}
		t.Fatalf("stream ended early: %v", scanner.Err())
		return "", nil
	}

	// The ready frame confirms the subscription is attached — only then is it
	// safe to post (a message sent before it could race the subscribe).
	if ev, data := nextFrame(); ev != "ready" || data["name"] != "researcher" {
		t.Fatalf("expected ready frame, got %s %v", ev, data)
	}

	// One match (DM to the watcher), two non-matches (foreign DM; the
	// watcher's own broadcast), one match (foreign broadcast).
	post(board.Message{Topic: "dm", From: "myapp", To: "researcher", Text: "deploy target?"})
	post(board.Message{Topic: "dm", From: "myapp", To: "writer", Text: "not yours"})
	post(board.Message{Topic: "broadcast", From: "researcher", To: board.Everyone, Text: "my own"})
	post(board.Message{Topic: "broadcast", From: "myapp", To: board.Everyone, Text: "heads-up"})

	ev, data := nextFrame()
	if ev != "mail" || data["text"] != "deploy target?" || data["to"] != "researcher" {
		t.Fatalf("first mail wrong: %s %v", ev, data)
	}
	ev, data = nextFrame()
	if ev != "mail" || data["text"] != "heads-up" {
		t.Fatalf("second mail wrong (filtered frames leaked?): %s %v", ev, data)
	}
}

// TestMailWatchMatch pins the filter table: name watching (directed,
// broadcast, own-broadcast, foreign), topic watching, and the firehose.
func TestMailWatchMatch(t *testing.T) {
	cases := []struct {
		name, topic string
		p           boardPostedPayload
		want        bool
	}{
		{"researcher", "", boardPostedPayload{To: "Researcher", From: "x"}, true},
		{"researcher", "", boardPostedPayload{To: "writer", From: "x"}, false},
		{"researcher", "", boardPostedPayload{To: "*", From: "myapp"}, true},
		{"researcher", "", boardPostedPayload{To: "*", From: "researcher"}, false},
		{"researcher", "", boardPostedPayload{To: "", Topic: "status"}, false},
		{"", "status", boardPostedPayload{Topic: "Status"}, true},
		{"", "status", boardPostedPayload{Topic: "dm"}, false},
		{"researcher", "dm", boardPostedPayload{To: "researcher", Topic: "status"}, false},
		{"", "", boardPostedPayload{Topic: "anything"}, true},
	}
	for i, c := range cases {
		if got := mailWatchMatch(c.name, c.topic, c.p); got != c.want {
			t.Errorf("case %d (%+v): got %v, want %v", i, c.p, got, c.want)
		}
	}
}
