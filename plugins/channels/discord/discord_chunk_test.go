// SPDX-License-Identifier: MIT

package discord

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

// A message longer than Discord's 2000-char limit must go out as several
// messages — a single oversize POST is rejected, losing the answer (M234).
func TestDiscord_SendChunksOverLongMessage(t *testing.T) {
	_, pub := keypair(t)
	posted := make(chan map[string]any, 16)
	api := discordAPI(t, posted)
	defer api.Close()

	c := New(Config{
		PublicKey: pub, Token: "bot-test", ApplicationID: "APP1",
		BaseURL:    api.URL,
		HTTPClient: api.Client(),
	})

	long := strings.Repeat("y", 5000) // 2.5× the limit
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "C1", Text: long}); err != nil {
		t.Fatalf("send: %v", err)
	}
	close(posted)

	var rejoined strings.Builder
	n := 0
	for m := range posted {
		n++
		txt, _ := m["content"].(string)
		if len([]rune(txt)) > discordMaxChars {
			t.Errorf("chunk %d is %d chars, over the %d limit", n, len([]rune(txt)), discordMaxChars)
		}
		rejoined.WriteString(txt)
	}
	if n < 3 {
		t.Fatalf("expected >=3 chunks for 5000 chars, got %d", n)
	}
	if rejoined.String() != long {
		t.Error("chunks did not rejoin to the original message")
	}
}
