// SPDX-License-Identifier: MIT

// OneBot channel: event parsing + CQ-message media helpers (validSignature + parseEvent + fetchMedia + extractCQMedia + cqUnescape).
// Code extracted from onebot.go during the Day-137 god-file split.
// Public API unchanged.
package onebot


import (
	"context"
	"io"
	"regexp"
	"strings"

	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
)

func validSignature(secret string, body []byte, header string) bool {
	if secret == "" || strings.TrimSpace(header) == "" {
		return false
	}
	header = strings.TrimSpace(strings.TrimPrefix(header, "sha1="))
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(want), []byte(header)) == 1
}

// parseEvent reads a OneBot v11 message event: {post_type:"message",
// message_type, message_id, user_id, group_id, raw_message/message}.
func parseEvent(body []byte) (inbound, bool) {
	var e struct {
		PostType    string      `json:"post_type"`
		MessageType string      `json:"message_type"`
		MessageID   json.Number `json:"message_id"`
		UserID      json.Number `json:"user_id"`
		GroupID     json.Number `json:"group_id"`
		RawMessage  string      `json:"raw_message"`
		Message     string      `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return inbound{}, false
	}
	if e.PostType != "" && e.PostType != "message" {
		return inbound{}, false
	}
	text := e.RawMessage
	if text == "" {
		text = e.Message
	}
	user := e.UserID.String()
	target := "private:" + user
	if e.MessageType == "group" && e.GroupID.String() != "" && e.GroupID.String() != "0" {
		target = "group:" + e.GroupID.String()
	}
	clean, media := extractCQMedia(text)
	return inbound{
		sender: user,
		target: target,
		text:   strings.TrimSpace(clean),
		id:     e.MessageID.String(),
		media:  media,
	}, true
}

var cqRe = regexp.MustCompile(`\[CQ:(image|record),([^\]]*)\]`)
var cqURLRe = regexp.MustCompile(`url=([^,\]]+)`)

// fetchMedia downloads a media URL referenced by a CQ code and returns it as an
// inline data: URL. Best-effort: returns "" on any failure.
func (c *Channel) fetchMedia(ctx context.Context, mediaURL string) string {
	if mediaURL == "" {
		return ""
	}
	// The URL is attacker-controlled (it arrived in an inbound CQ code). Require
	// http(s) and fetch only through the SSRF-guarded client, which rejects
	// loopback/private/link-local/metadata targets on the initial dial and on
	// every redirect hop.
	u, err := url.Parse(mediaURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return ""
	}
	resp, err := c.mediaClient.Do(req)
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
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// extractCQMedia pulls image/record media URLs out of a OneBot raw_message's CQ
// codes and returns the text with those codes removed. CQ entity escapes are
// decoded for the URL.
func extractCQMedia(raw string) (string, []obMedia) {
	var media []obMedia
	for _, m := range cqRe.FindAllStringSubmatch(raw, -1) {
		kind := "image"
		if m[1] == "record" {
			kind = "audio"
		}
		if u := cqURLRe.FindStringSubmatch(m[2]); u != nil {
			media = append(media, obMedia{kind: kind, url: cqUnescape(u[1])})
		}
	}
	return cqRe.ReplaceAllString(raw, ""), media
}

func cqUnescape(s string) string {
	r := strings.NewReplacer("&amp;", "&", "&#91;", "[", "&#93;", "]", "&#44;", ",")
	return r.Replace(s)
}
