// SPDX-License-Identifier: MIT

package wecom

// WeCom outbound transport: Send + sendOne + fetchMedia + accessToken.
// Carved out of wecom.go during the Day 146 god-file split so the main file
// can focus on lifecycle + inbound HTTP webhook handling.
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
	text := strings.TrimSpace(out.Text)
	if target == "" {
		return fmt.Errorf("wecom: send requires a user id")
	}
	if text == "" {
		return nil
	}
	tok, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	for _, chunk := range channel.SplitText(text, wecomMaxChars) {
		if err := c.sendOne(ctx, tok, target, chunk); err != nil {
			return err
		}
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel.outbound.wecom", Kind: event.KindChannelOutbound, Actor: "channel-wecom",
			Payload: map[string]any{"channel_kind": "wecom", "channel_id": target, "text": text},
		})
	}
	return nil
}

func (c *Channel) sendOne(ctx context.Context, token, user, text string) error {
	payload := map[string]any{
		"touser":  user,
		"msgtype": "text",
		"agentid": c.cfg.AgentID,
		"text":    map[string]string{"content": text},
	}
	raw, _ := json.Marshal(payload)
	url := c.apiBase + "/cgi-bin/message/send?access_token=" + token
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
		return fmt.Errorf("wecom: send returned status %d", resp.StatusCode)
	}
	return nil
}

// fetchMedia downloads an inbound media file by id (cgi-bin/media/get) and
// returns it as an inline data: URL. Best-effort: returns "" on any failure.
func (c *Channel) fetchMedia(ctx context.Context, mediaID, mediaType string) string {
	if mediaID == "" {
		return ""
	}
	tok, err := c.accessToken(ctx)
	if err != nil || tok == "" {
		return ""
	}
	endpoint := c.apiBase + "/cgi-bin/media/get?access_token=" + url.QueryEscape(tok) + "&media_id=" + url.QueryEscape(mediaID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
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
	// WeCom returns JSON ({"errcode":...}) on failure rather than binary.
	if len(data) > 0 && data[0] == '{' {
		return ""
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		if mediaType == "audio" {
			mime = "audio/amr"
		} else {
			mime = "image/jpeg"
		}
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func (c *Channel) accessToken(ctx context.Context) (string, error) {
	c.tmu.Lock()
	defer c.tmu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}
	url := fmt.Sprintf("%s/cgi-bin/gettoken?corpid=%s&corpsecret=%s", c.apiBase, c.cfg.CorpID, c.cfg.CorpSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	var tr struct {
		ErrCode int    `json:"errcode"`
		Token   string `json:"access_token"`
		Exp     int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if tr.Token == "" {
		return "", fmt.Errorf("wecom: token fetch failed (errcode %d)", tr.ErrCode)
	}
	c.token = tr.Token
	exp := tr.Exp
	if exp <= 0 {
		exp = 7200
	}
	c.tokenExp = time.Now().Add(time.Duration(exp-60) * time.Second)
	return c.token, nil
}
