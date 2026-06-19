// SPDX-License-Identifier: MIT

package feishu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

func TestURLVerification(t *testing.T) {
	ch, tok, ok := urlVerification([]byte(`{"challenge":"abc","token":"t1","type":"url_verification"}`))
	if !ok || ch != "abc" || tok != "t1" {
		t.Fatalf("urlVerification = %q %q %v", ch, tok, ok)
	}
	if _, _, ok := urlVerification([]byte(`{"type":"event_callback"}`)); ok {
		t.Fatal("non-verification should be false")
	}
}

func TestParseEvent(t *testing.T) {
	body := []byte(`{"header":{"token":"t1","event_id":"E1","event_type":"im.message.receive_v1"},"event":{"sender":{"sender_id":{"open_id":"ou_1"}},"message":{"message_id":"m1","chat_id":"oc_1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`)
	m, tok, ok := parseEvent(body)
	if !ok || tok != "t1" || m.sender != "ou_1" || m.chatID != "oc_1" || m.text != "hi" || m.id != "m1" {
		t.Fatalf("parseEvent = %+v tok=%q ok=%v", m, tok, ok)
	}
	// image message is kept with its image_key + media type so the resource can be fetched.
	im, _, ok := parseEvent([]byte(`{"header":{"event_type":"im.message.receive_v1"},"event":{"message":{"message_id":"m2","message_type":"image","content":"{\"image_key\":\"img_k1\"}"}}}`))
	if !ok || im.mediaType != "image" || im.fileKey != "img_k1" || im.messageID != "m2" {
		t.Fatalf("image parseEvent = %+v ok=%v", im, ok)
	}
	// audio message keeps its file_key under the audio media type.
	au, _, ok := parseEvent([]byte(`{"header":{"event_type":"im.message.receive_v1"},"event":{"message":{"message_id":"m3","message_type":"audio","content":"{\"file_key\":\"file_k1\"}"}}}`))
	if !ok || au.mediaType != "audio" || au.fileKey != "file_k1" {
		t.Fatalf("audio parseEvent = %+v ok=%v", au, ok)
	}
	// genuinely unsupported message types are still dropped.
	if _, _, ok := parseEvent([]byte(`{"header":{"event_type":"im.message.receive_v1"},"event":{"message":{"message_type":"sticker"}}}`)); ok {
		t.Fatal("sticker message should be dropped")
	}
}

func TestSendFetchesTokenThenPosts(t *testing.T) {
	var tokenHits, sendHits int
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "tenant_access_token"):
			tokenHits++
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "T-123", "expire": 7200})
		case strings.Contains(r.URL.Path, "/im/v1/messages"):
			sendHits++
			auth = r.Header.Get("Authorization")
			b, _ := io.ReadAll(r.Body)
			var p map[string]any
			_ = json.Unmarshal(b, &p)
			if p["receive_id"] != "oc_1" {
				t.Errorf("receive_id = %v", p["receive_id"])
			}
		}
	}))
	defer srv.Close()
	ch := New(Config{AppID: "a", AppSecret: "s", APIBase: srv.URL, HTTPClient: srv.Client()})
	if err := ch.Send(context.Background(), channel.Outbound{ChannelID: "oc_1", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if tokenHits != 1 || sendHits != 1 || auth != "Bearer T-123" {
		t.Fatalf("tokenHits=%d sendHits=%d auth=%q", tokenHits, sendHits, auth)
	}
	// second send reuses the cached token.
	if err := ch.Send(context.Background(), channel.Outbound{ChannelID: "oc_1", Text: "again"}); err != nil {
		t.Fatal(err)
	}
	if tokenHits != 1 {
		t.Fatalf("token not cached, tokenHits=%d", tokenHits)
	}
}
