// SPDX-License-Identifier: MIT
//
// iMessage channel: sendOne + sendAttachment (low-level POST helpers used
// by Channel.Send).
// Extracted from imessage.go during the Day-202 god-file split.
// Public API unchanged.
package imessage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/ulid"
)

func (c *Channel) sendOne(ctx context.Context, guid, text string) error {
	endpoint := c.base + "/api/v1/message/text"
	if c.cfg.Password != "" {
		endpoint += "?password=" + url.QueryEscape(c.cfg.Password)
	}
	payload := map[string]any{
		"chatGuid": guid,
		"tempGuid": "agezt-" + ulid.New(),
		"message":  text,
		"method":   c.method,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return scrubURLError(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("imessage: BlueBubbles returned status %d", resp.StatusCode)
	}
	return nil
}

// sendAttachment uploads one media attachment to a chat via BlueBubbles'
// /api/v1/message/attachment endpoint (multipart/form-data).
func (c *Channel) sendAttachment(ctx context.Context, guid string, att channel.Attachment) error {
	if len(att.Data) == 0 {
		return nil
	}
	fn := att.Filename
	if fn == "" {
		fn = "attachment"
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chatGuid", guid)
	_ = mw.WriteField("tempGuid", "agezt-"+ulid.New())
	_ = mw.WriteField("name", fn)
	_ = mw.WriteField("method", "private-api")
	fw, err := mw.CreateFormFile("attachment", fn)
	if err != nil {
		return err
	}
	if _, err := fw.Write(att.Data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	endpoint := c.base + "/api/v1/message/attachment"
	if c.cfg.Password != "" {
		endpoint += "?password=" + url.QueryEscape(c.cfg.Password)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.client.Do(req)
	if err != nil {
		return scrubURLError(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("imessage: attachment upload returned status %d", resp.StatusCode)
	}
	return nil
}

