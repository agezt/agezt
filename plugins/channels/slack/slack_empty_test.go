// SPDX-License-Identifier: MIT

package slack

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/channel"
)

// An empty or whitespace-only message is a no-op, not a failed chat.postMessage
// (Slack errors with "no_text") — covers the Send path and whitespace answers
// (M236).
func TestSlack_EmptySendIsNoOp(t *testing.T) {
	posted := make(chan map[string]any, 4)
	api := slackAPI(t, posted)
	defer api.Close()

	c := New(Config{
		Token: "xoxb-test", BaseURL: api.URL, HTTPClient: api.Client(),
	})
	for _, txt := range []string{"", "  ", "\n\t"} {
		if err := c.Send(context.Background(), channel.Outbound{ChannelID: "C1", Text: txt}); err != nil {
			t.Fatalf("empty send should be a no-op (nil), got %v", err)
		}
	}
	select {
	case m := <-posted:
		t.Fatalf("empty send should post nothing, got %v", m)
	case <-time.After(300 * time.Millisecond):
	}
}
