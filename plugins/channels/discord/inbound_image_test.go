// SPDX-License-Identifier: MIT

package discord

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/channel"
)

// A slash command carrying an ATTACHMENT image option resolves the attachment,
// downloads it, and hands it to the handler as a data: URL (M249).
func TestDiscord_AttachmentImageBecomesDataURL(t *testing.T) {
	priv, pub := keypair(t)
	posted := make(chan map[string]any, 1)
	api := discordAPI(t, posted)
	defer api.Close()

	raw := []byte{0x89, 0x50, 0x4e, 0x47, 9, 8, 7}
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(raw)
	}))
	defer cdn.Close()

	var gotImages atomic.Value
	c := New(Config{
		PublicKey: pub, Token: "bot-test", ApplicationID: "APP1",
		BaseURL:    api.URL,
		HTTPClient: api.Client(),
		Allowlist:  channel.NewAllowlist([]string{"C1"}),
		Handler: func(_ context.Context, msg channel.UnifiedMessage, _ string) (channel.Reply, error) {
			gotImages.Store(msg.Images)
			return channel.Reply{Text: "seen"}, nil
		},
	})
	// The attachment is served by a local httptest "cdn"; relax the H-001 host
	// policy for this end-to-end image-flow test (host policy is unit-tested
	// separately in TestValidDiscordAttachmentURL).
	c.attachURLOK = func(string) error { return nil }

	attURL := cdn.URL + "/attachments/pic.png"
	body := []byte(fmt.Sprintf(`{"type":2,"id":"I1","token":"tok","channel_id":"C1","member":{"user":{"id":"U1"}},`+
		`"data":{"name":"agezt","options":[{"name":"image","type":11,"value":"att1"}],`+
		`"resolved":{"attachments":{"att1":{"url":%q,"content_type":"image/png","filename":"pic.png"}}}}}`, attURL))
	if rec := postInteraction(t, c, priv, body, false, ""); rec.Code != 200 {
		t.Fatalf("ACK code = %d want 200", rec.Code)
	}

	select {
	case m := <-posted:
		_ = m // follow-up delivered → the async run finished
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for follow-up")
	}

	imgs, _ := gotImages.Load().([]string)
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	if len(imgs) != 1 || imgs[0] != want {
		t.Errorf("Images=%v\nwant [%s]", imgs, want)
	}
}

// imageAttachments only returns options that are ATTACHMENT type and resolve to
// an image; a non-image attachment and a missing resolved map yield nothing.
func TestDiscord_ImageAttachmentsFilter(t *testing.T) {
	mk := func(j string) discordInteraction {
		var in discordInteraction
		if err := json.Unmarshal([]byte(j), &in); err != nil {
			t.Fatalf("bad json: %v", err)
		}
		return in
	}

	img := mk(`{"data":{"options":[{"name":"image","type":11,"value":"a"}],"resolved":{"attachments":{"a":{"url":"u","content_type":"image/png"}}}}}`)
	if got := img.imageAttachments(); len(got) != 1 || got[0].URL != "u" {
		t.Errorf("image case = %v, want one attachment", got)
	}

	nonImage := mk(`{"data":{"options":[{"name":"file","type":11,"value":"a"}],"resolved":{"attachments":{"a":{"url":"u","content_type":"application/pdf"}}}}}`)
	if got := nonImage.imageAttachments(); len(got) != 0 {
		t.Errorf("non-image case = %v, want none", got)
	}

	noResolved := mk(`{"data":{"options":[{"name":"image","type":11,"value":"a"}]}}`)
	if got := noResolved.imageAttachments(); len(got) != 0 {
		t.Errorf("no-resolved case = %v, want none", got)
	}

	stringOpt := mk(`{"data":{"options":[{"name":"prompt","type":3,"value":"hi"}]}}`)
	if got := stringOpt.imageAttachments(); len(got) != 0 {
		t.Errorf("string-option case = %v, want none", got)
	}
}
