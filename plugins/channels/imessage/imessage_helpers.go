// SPDX-License-Identifier: MIT

// iMessage channel: helpers (scrubURLError + fetchAttachmentData + seenBefore + parseWebhook + chatGUID).
// Code extracted from imessage.go during the Day-102 god-file split.
// Public API unchanged.
package imessage


import (
	"context"
	"io"
	"strings"

	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
)

func (c *Channel) fetchAttachmentData(ctx context.Context, a imAttachment) string {
	endpoint := c.base + "/api/v1/attachment/" + url.PathEscape(a.guid) + "/download"
	if c.cfg.Password != "" {
		endpoint += "?password=" + url.QueryEscape(c.cfg.Password)
	}
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
	mime := a.mime
	if mime == "" {
		mime = resp.Header.Get("Content-Type")
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// seenBefore reports whether a message guid was already processed (replay guard),
// recording it otherwise. Bounded by a small ring.
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

// parseWebhook reads a BlueBubbles "new-message" webhook:
// {type, data:{guid, text, isFromMe, handle:{address}, chats:[{guid}]}}.
func parseWebhook(body []byte) (inbound, bool) {
	var w struct {
		Type string `json:"type"`
		Data struct {
			GUID     string `json:"guid"`
			Text     string `json:"text"`
			IsFromMe bool   `json:"isFromMe"`
			Handle   struct {
				Address string `json:"address"`
			} `json:"handle"`
			Chats []struct {
				GUID string `json:"guid"`
			} `json:"chats"`
			Attachments []struct {
				GUID         string `json:"guid"`
				MimeType     string `json:"mimeType"`
				TransferName string `json:"transferName"`
			} `json:"attachments"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return inbound{}, false
	}
	if w.Type != "" && w.Type != "new-message" {
		return inbound{}, false
	}
	if w.Data.IsFromMe {
		return inbound{}, false
	}
	chat := ""
	if len(w.Data.Chats) > 0 {
		chat = w.Data.Chats[0].GUID
	}
	var atts []imAttachment
	for _, a := range w.Data.Attachments {
		if a.GUID == "" {
			continue
		}
		atts = append(atts, imAttachment{guid: a.GUID, mime: a.MimeType, name: a.TransferName})
	}
	return inbound{
		chatGUID:    chat,
		sender:      w.Data.Handle.Address,
		text:        w.Data.Text,
		id:          w.Data.GUID,
		attachments: atts,
	}, true
}

// chatGUID normalizes a send target: a full BlueBubbles chat guid (contains ';')
// is used verbatim; a bare phone/email is wrapped as a direct iMessage guid.
func chatGUID(target string) string {
	if strings.Contains(target, ";") {
		return target
	}
	return "iMessage;-;" + target
}
