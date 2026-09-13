// SPDX-License-Identifier: MIT

package telegram

// Telegram outbound Send path: Send + send + fetchPhotoDataURL +
// tgMediaType. Carved out of telegram.go during the Day 188 god-file
// split so the main file can stay focused on Channel/Config types +
// lifecycle and the inbound file can stay focused on getUpdates +
// handleInbound.
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
	"path/filepath"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
)

func (c *Channel) fetchPhotoDataURL(ctx context.Context, fileID string) (string, error) {
	gf := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", c.base, c.token, url.QueryEscape(fileID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gf, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", c.scrubToken(err)
	}
	var gfResp struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := func() error {
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("telegram getFile: status %d", resp.StatusCode)
		}
		return json.NewDecoder(io.LimitReader(resp.Body, tgAPIMaxResponseBytes)).Decode(&gfResp)
	}(); err != nil {
		return "", err
	}
	if !gfResp.OK || gfResp.Result.FilePath == "" {
		return "", fmt.Errorf("telegram getFile: no file_path")
	}

	dl := fmt.Sprintf("%s/file/bot%s/%s", c.base, c.token, gfResp.Result.FilePath)
	dreq, err := http.NewRequestWithContext(ctx, http.MethodGet, dl, nil)
	if err != nil {
		return "", err
	}
	dresp, err := c.client.Do(dreq)
	if err != nil {
		return "", c.scrubToken(err)
	}
	defer dresp.Body.Close()
	if dresp.StatusCode/100 != 2 {
		return "", fmt.Errorf("telegram file download: status %d", dresp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(dresp.Body, tgPhotoMaxRaw+1))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("telegram file download: empty")
	}
	if len(data) > tgPhotoMaxRaw {
		return "", fmt.Errorf("telegram photo exceeds %d bytes", tgPhotoMaxRaw)
	}
	return "data:" + tgMediaType(gfResp.Result.FilePath) + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// tgMediaType maps a Telegram file_path extension to an image media type.
// Telegram photos are JPEG; stickers/other uploads may differ.
func tgMediaType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".oga", ".ogg":
		return "audio/ogg"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	default:
		return "image/jpeg"
	}
}

// Send implements channel.Channel (used by the Pulse→Telegram sink and any
// out-of-band sender). Journaled under no correlation.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	return c.send(ctx, out, "")
}

// telegramMaxChars is Telegram's per-message limit (4096 UTF-16 code units).
// A longer sendMessage is rejected with 400, so a long answer is split into
// sequential messages rather than lost (M234).
const telegramMaxChars = 4096

// send POSTs sendMessage (chunked to the platform limit) and journals
// channel.outbound under corr.
func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	// An empty or whitespace-only message is rejected by Telegram (400
	// "message text is empty"). Treat it as a no-op rather than a failed send —
	// covers the Send path (Pulse, agt send) and whitespace-only agent answers
	// the inbound reply guard's exact-"" check would miss (M236).
	if strings.TrimSpace(out.Text) == "" && len(out.Attachments) == 0 {
		return nil
	}
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", c.base, c.token)
	for _, chunk := range channel.SplitText(out.Text, telegramMaxChars) {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		fields := map[string]any{"chat_id": out.ChannelID, "text": chunk}
		// Forum-topic reply (M885): message_thread_id routes the message into
		// the originating topic instead of the chat's General stream.
		if out.ThreadID != "" {
			if tid, err := strconv.ParseInt(out.ThreadID, 10, 64); err == nil {
				fields["message_thread_id"] = tid
			}
		}
		body, _ := json.Marshal(fields)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.client.Do(req)
		if err != nil {
			return c.scrubToken(err)
		}
		// Drain+close before the next iteration so the connection is reused.
		err = func() error {
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				return fmt.Errorf("telegram sendMessage: status %d", resp.StatusCode)
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	// Outbound media: voice clips, photos, files (M-multimodal).
	for _, att := range out.Attachments {
		if err := c.sendAttachment(ctx, out, att); err != nil {
			return err
		}
	}
	c.emitOutbound(out, corr)
	return nil
}

// sendAttachment uploads one media attachment via the matching Bot API method
// (sendVoice for OGG/Opus, sendAudio for other audio, sendPhoto for images,
// sendDocument otherwise) as multipart/form-data.

