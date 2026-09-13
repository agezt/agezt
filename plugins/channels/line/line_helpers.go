// SPDX-License-Identifier: MIT

package line

// LINE helpers: emitInbound + seenBefore + textMessages +
// fetchContent + validSignature + parseWebhook. Carved out of
// line.go during the Day 191 god-file split so the main file can
// stay focused on Config/Channel types + lifecycle + inbound
// webhook handling and the send file can stay focused on
// outbound Send.
// Public API unchanged.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
)

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound.line",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-line",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "line", "channel_id": msg.ChannelID,
			"sender": msg.Sender, "text": msg.Text, "allowed": allowed,
		},
	})
}

func (c *Channel) seenBefore(id string) bool {
	c.dmu.Lock()
	defer c.dmu.Unlock()
	if _, ok := c.seen[id]; ok {
		return true
	}
	c.seen[id] = struct{}{}
	c.ring = append(c.ring, id)
	if len(c.ring) > dedupCapacity {
		old := c.ring[0]
		c.ring = c.ring[1:]
		delete(c.seen, old)
	}
	return false
}

// ---- wire shapes ---------------------------------------------------------

// textMessages splits text into LINE message objects (max 5 per request, each
// ≤5000 chars).
func textMessages(text string) []map[string]any {
	chunks := channel.SplitText(text, lineMaxChars)
	if len(chunks) > 5 {
		chunks = chunks[:5]
	}
	msgs := make([]map[string]any, 0, len(chunks))
	for _, ch := range chunks {
		msgs = append(msgs, map[string]any{"type": "text", "text": ch})
	}
	return msgs
}

// fetchContent downloads a LINE message's binary content (image/audio) from the
// content host and returns it as an inline data: URL. Best-effort: returns ""
// on any failure.
func (c *Channel) fetchContent(ctx context.Context, messageID, kind string) string {
	if messageID == "" {
		return ""
	}
	base := c.apiBase
	if base == defaultAPIBase {
		base = "https://api-data.line.me" // LINE serves content from a separate host
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v2/bot/message/"+messageID+"/content", nil)
	if err != nil {
		return ""
	}
	if c.cfg.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil || len(data) == 0 || len(data) > 16<<20 {
		return ""
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		if kind == "audio" {
			mime = "audio/m4a"
		} else {
			mime = "image/jpeg"
		}
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// validSignature checks X-Line-Signature = base64(HMAC-SHA256(secret, body)).
// An empty secret fails closed (no unsigned inbound). buildLine already refuses
// to construct the two-way channel without a secret; this keeps the invariant in
// the layer that enforces it, not only in the layer that configures it.
func validSignature(secret string, body []byte, header string) bool {
	if secret == "" || strings.TrimSpace(header) == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(want), []byte(strings.TrimSpace(header))) == 1
}

// parseWebhook reads a LINE webhook: {events:[{type, replyToken, source:{userId},
// message:{type, id, text}}]}. Only text messages from users are kept.
func parseWebhook(body []byte) []inbound {
	var w struct {
		Events []struct {
			Type       string `json:"type"`
			ReplyToken string `json:"replyToken"`
			Source     struct {
				UserID  string `json:"userId"`
				GroupID string `json:"groupId"`
			} `json:"source"`
			Message struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"message"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return nil
	}
	var out []inbound
	for _, e := range w.Events {
		if e.Type != "message" {
			continue
		}
		from := e.Source.UserID
		if from == "" {
			from = e.Source.GroupID
		}
		in := inbound{userID: from, replyToken: e.ReplyToken, id: e.Message.ID}
		switch e.Message.Type {
		case "text":
			in.text = e.Message.Text
		case "image":
			in.mediaType = "image"
		case "audio":
			in.mediaType = "audio"
		default:
			continue
		}
		out = append(out, in)
	}
	return out
}

