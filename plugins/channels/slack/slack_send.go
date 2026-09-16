package slack

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
)


func (c *Channel) sendFile(ctx context.Context, channelID, threadTS string, att channel.Attachment) error {
	if len(att.Data) == 0 {
		return nil
	}
	fn := att.Filename
	if fn == "" {
		fn = "file"
	}
	// 1) get an upload URL + file id.
	form := url.Values{"filename": {fn}, "length": {strconv.Itoa(len(att.Data))}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/files.getUploadURLExternal", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	var up struct {
		OK        bool   `json:"ok"`
		Error     string `json:"error"`
		UploadURL string `json:"upload_url"`
		FileID    string `json:"file_id"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&up)
	resp.Body.Close()
	if err != nil {
		return err
	}
	if !up.OK || up.UploadURL == "" {
		return fmt.Errorf("slack getUploadURLExternal: %s", up.Error)
	}
	// 2) POST the bytes to the upload URL (multipart field "file").
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", fn)
	if err != nil {
		return err
	}
	if _, err := fw.Write(att.Data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	ureq, err := http.NewRequestWithContext(ctx, http.MethodPost, up.UploadURL, &buf)
	if err != nil {
		return err
	}
	ureq.Header.Set("Content-Type", mw.FormDataContentType())
	uresp, err := c.client.Do(ureq)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(uresp.Body, 8<<10))
	uresp.Body.Close()
	if uresp.StatusCode/100 != 2 {
		return fmt.Errorf("slack upload: status %d", uresp.StatusCode)
	}
	// 3) complete + share into the channel/thread.
	complete := map[string]any{
		"files":      []map[string]string{{"id": up.FileID, "title": fn}},
		"channel_id": channelID,
	}
	if threadTS != "" {
		complete["thread_ts"] = threadTS
	}
	cbody, _ := json.Marshal(complete)
	creq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/files.completeUploadExternal", bytes.NewReader(cbody))
	if err != nil {
		return err
	}
	creq.Header.Set("Content-Type", "application/json; charset=utf-8")
	creq.Header.Set("Authorization", "Bearer "+c.token)
	cresp, err := c.client.Do(creq)
	if err != nil {
		return err
	}
	defer cresp.Body.Close()
	var cr struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(cresp.Body, maxBody)).Decode(&cr); err != nil {
		return err
	}
	if !cr.OK {
		return fmt.Errorf("slack completeUploadExternal: %s", cr.Error)
	}
	return nil
}

// postMessage delivers one chat.postMessage and verifies Slack's app-level ok.
// Slack returns HTTP 200 even on application errors ({"ok":false,"error":…}), so
// the body must be decoded and ok checked — a decode failure or ok=false is a
// FAILED send, not a delivered one.
func (c *Channel) postMessage(ctx context.Context, channelID, threadTS, text string) error {
	fields := map[string]any{"channel": channelID, "text": text}
	if threadTS != "" {
		fields["thread_ts"] = threadTS // M885: reply inside the originating thread
	}
	body, _ := json.Marshal(fields)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("slack chat.postMessage: status %d", resp.StatusCode)
	}
	var pm postMessageResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&pm); err != nil {
		return fmt.Errorf("slack chat.postMessage: decode response: %w", err)
	}
	if !pm.OK {
		reason := pm.Error
		if reason == "" {
			reason = "ok=false"
		}
		return fmt.Errorf("slack chat.postMessage: %s", reason)
	}
	return nil
}

// --- journaling ------------------------------------------------------------

// slackFileMaxRaw bounds a downloaded file so the data: URL stays within the
// control-plane request cap (16 MiB; base64 ≈ 4/3 × raw).
const slackFileMaxRaw = 12 << 20

// fetchFileDataURL downloads a Slack url_private attachment (which requires the
// bot token in an Authorization header) and returns it as an inline data: URL.
// The bytes are read here in the channel, where the token lives, and handed
// onward as a self-describing data: URL the vision providers emit natively
// (M248).
func (c *Channel) fetchFileDataURL(ctx context.Context, urlPrivate, mimetype string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPrivate, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("slack file download: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, slackFileMaxRaw+1))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("slack file download: empty")
	}
	if len(data) > slackFileMaxRaw {
		return "", fmt.Errorf("slack file exceeds %d bytes", slackFileMaxRaw)
	}
	return "data:" + mimetype + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}


// slackTSMillis converts a Slack ts ("1700000000.000100") to unix millis; 0 on
// parse failure.
func slackTSMillis(ts string) int64 {
	f, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return 0
	}
	return int64(f * 1000)
}

// ============================================================================

// Send + send + postMessageResp type. Carved out of slack.go during
// the Day 200 god-file split so the main file can stay focused on
// types + lifecycle + verify and the inbound file can stay focused
// on the receive flow.

func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	return c.send(ctx, out, "")
}

type postMessageResp struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// slackMaxChars is Slack's per-message text limit (40000 characters). A longer
// message is rejected, so a long answer is split into sequential messages rather
// than lost (M240) — same treatment as Telegram/Discord (M234/M235).
const slackMaxChars = 40000

func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	// Slack rejects an empty message (errors with "no_text"); no-op rather than
	// fail, covering the Send path and whitespace-only answers (M236).
	if strings.TrimSpace(out.Text) == "" && len(out.Attachments) == 0 {
		return nil
	}
	for _, chunk := range channel.SplitText(out.Text, slackMaxChars) {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		if err := c.postMessage(ctx, out.ChannelID, out.ThreadID, chunk); err != nil {
			return err
		}
	}
	for _, att := range out.Attachments {
		if err := c.sendFile(ctx, out.ChannelID, out.ThreadID, att); err != nil {
			return err
		}
	}
	c.emitOutbound(out, corr)
	return nil
}

// sendFile uploads an attachment via Slack's external-upload flow:
// files.getUploadURLExternal → POST bytes to the upload URL →
// files.completeUploadExternal (which shares it into the channel/thread).
