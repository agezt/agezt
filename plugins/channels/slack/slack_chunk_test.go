// SPDX-License-Identifier: MIT

package slack

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

// A message longer than Slack's 40000-char limit goes out as several messages
// rather than being rejected — same treatment as Telegram/Discord (M240).
func TestSlack_SendChunksOverLongMessage(t *testing.T) {
	posted := make(chan map[string]any, 8)
	api := slackAPI(t, posted)
	defer api.Close()

	c := New(Config{Token: "xoxb-test", BaseURL: api.URL, HTTPClient: api.Client()})

	long := strings.Repeat("y", 90000) // > 2× the limit
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "C1", Text: long}); err != nil {
		t.Fatalf("send: %v", err)
	}
	close(posted)

	var rejoined strings.Builder
	n := 0
	for m := range posted {
		n++
		txt, _ := m["text"].(string)
		if len([]rune(txt)) > slackMaxChars {
			t.Errorf("chunk %d is %d chars, over the %d limit", n, len([]rune(txt)), slackMaxChars)
		}
		rejoined.WriteString(txt)
	}
	if n < 2 {
		t.Fatalf("expected >=2 chunks for 90000 chars, got %d", n)
	}
	if rejoined.String() != long {
		t.Error("chunks did not rejoin to the original message")
	}
}
