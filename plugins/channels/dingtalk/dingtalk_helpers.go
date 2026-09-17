// SPDX-License-Identifier: MIT

// dingtalk_helpers.go: post + emitInbound + seenBefore + validSign + safeReplyURL
// + parseInbound split off from dingtalk.go during the Day 211 god-file refactor (#142).
// Public API unchanged.
package dingtalk

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
)

func (c *Channel) post(ctx context.Context, url, text string) error {
	if url == "" {
		return fmt.Errorf("dingtalk: no reply URL")
	}
	payload := map[string]any{"msgtype": "text", "text": map[string]any{"content": text}}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("dingtalk: webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound.dingtalk",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-dingtalk",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "dingtalk", "channel_id": msg.ChannelID,
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

// validSign verifies DingTalk's outgoing signature:
// sign = base64(HMAC-SHA256(secret, timestamp + "\n" + secret)). The timestamp
// must be within ±5 minutes of now (DingTalk's window) to block replay. An
// empty secret fails closed (no unsigned inbound).
func validSign(secret, timestamp, sign string) bool {
	if secret == "" || timestamp == "" || sign == "" {
		return false
	}
	ms, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if d := time.Now().UnixMilli() - ms; d > 5*60*1000 || d < -5*60*1000 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "\n" + secret))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(want), []byte(strings.TrimSpace(sign))) == 1
}

// safeReplyURL reports whether raw is a genuine DingTalk session webhook
// (https + a *.dingtalk.com host) — used to refuse SSRF to arbitrary hosts.
func safeReplyURL(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "oapi.dingtalk.com" || strings.HasSuffix(host, ".dingtalk.com")
}

// parseInbound reads a DingTalk robot message: {msgtype, text:{content},
// senderStaffId, senderNick, msgId, sessionWebhook}.
func parseInbound(body []byte) (inbound, bool) {
	var d struct {
		MsgType string `json:"msgtype"`
		Text    struct {
			Content string `json:"content"`
		} `json:"text"`
		SenderStaffID  string `json:"senderStaffId"`
		SenderNick     string `json:"senderNick"`
		MsgID          string `json:"msgId"`
		SessionWebhook string `json:"sessionWebhook"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return inbound{}, false
	}
	if d.MsgType != "" && d.MsgType != "text" {
		return inbound{}, false
	}
	sender := d.SenderStaffID
	if sender == "" {
		sender = d.SenderNick
	}
	return inbound{
		sender:   sender,
		text:     strings.TrimSpace(d.Text.Content),
		id:       d.MsgID,
		replyURL: d.SessionWebhook,
	}, true
}
