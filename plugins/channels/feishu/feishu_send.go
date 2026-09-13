// SPDX-License-Identifier: MIT

package feishu

// Feishu OUTBOUND Send path: Send + sendOne + fetchResource +
// tenantToken. Carved out of feishu.go during the Day 193 god-file
// split so the main file can stay focused on types + lifecycle +
// inbound handling and the helpers file can stay focused on
// emit/seen helpers.
// Public API unchanged.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
)

func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	target := strings.TrimSpace(out.ChannelID)
	if target == "" {
		target = c.cfg.DefaultChat
	}
	text := strings.TrimSpace(out.Text)
	if target == "" {
		return fmt.Errorf("feishu: send requires a chat_id")
	}
	if text == "" {
		return nil
	}
	tok, err := c.tenantToken(ctx)
	if err != nil {
		return err
	}
	for _, chunk := range channel.SplitText(text, feishuMaxChars) {
		if err := c.sendOne(ctx, tok, target, chunk); err != nil {
			return err
		}
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel.outbound.feishu", Kind: event.KindChannelOutbound, Actor: "channel-feishu",
			Payload: map[string]any{"channel_kind": "feishu", "channel_id": target, "text": text},
		})
	}
	return nil
}

func (c *Channel) sendOne(ctx context.Context, token, chatID, text string) error {
	content, _ := json.Marshal(map[string]string{"text": text})
	payload := map[string]any{"receive_id": chatID, "msg_type": "text", "content": string(content)}
	raw, _ := json.Marshal(payload)
	url := c.apiBase + "/open-apis/im/v1/messages?receive_id_type=chat_id"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("feishu: send returned status %d", resp.StatusCode)
	}
	return nil
}

// fetchResource downloads an inbound message resource (image/file) and returns
// it as an inline data: URL. Best-effort: returns "" on any failure.
func (c *Channel) fetchResource(ctx context.Context, messageID, fileKey, mediaType string) string {
	if messageID == "" || fileKey == "" {
		return ""
	}
	token, err := c.tenantToken(ctx)
	if err != nil || token == "" {
		return ""
	}
	rtype := "file"
	if mediaType == "image" {
		rtype = "image"
	}
	endpoint := c.apiBase + "/open-apis/im/v1/messages/" + url.PathEscape(messageID) +
		"/resources/" + url.PathEscape(fileKey) + "?type=" + rtype
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
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
		if mediaType == "audio" {
			mime = "audio/opus"
		} else {
			mime = "image/jpeg"
		}
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// tenantToken returns a cached tenant_access_token, fetching a fresh one when
// expired.
func (c *Channel) tenantToken(ctx context.Context) (string, error) {
	c.tmu.Lock()
	defer c.tmu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}
	payload, _ := json.Marshal(map[string]string{"app_id": c.cfg.AppID, "app_secret": c.cfg.AppSecret})
	url := c.apiBase + "/open-apis/auth/v3/tenant_access_token/internal"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	var tr struct {
		Code  int    `json:"code"`
		Token string `json:"tenant_access_token"`
		Exp   int    `json:"expire"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if tr.Token == "" {
		return "", fmt.Errorf("feishu: token fetch failed (code %d)", tr.Code)
	}
	c.token = tr.Token
	exp := tr.Exp
	if exp <= 0 {
		exp = 7200
	}
	c.tokenExp = time.Now().Add(time.Duration(exp-60) * time.Second)
	return c.token, nil
}

