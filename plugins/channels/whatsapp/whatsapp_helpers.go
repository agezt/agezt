// SPDX-License-Identifier: MIT

// whatsapp_helpers.go: media-fetcher helper split off from whatsapp.go
// during the Day 211 god-file refactor (#126). Public API unchanged.
package whatsapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)


// waMediaMaxRaw bounds a downloaded media blob so the resulting data: URL stays
// within reason (voice notes are small; 16 MiB is generous).
const waMediaMaxRaw = 16 << 20

// fetchMediaDataURL resolves a WhatsApp Cloud API media id to an inline data:
// URL. Two calls: GET the media id to learn its (short-lived, authed) download
// URL + MIME, then download the bytes with the same bearer token. The daemon
// holds the access token (not the provider), so the bytes are fetched here and
// handed onward as a self-describing data: URL — same as the Telegram path.
func (c *Channel) fetchMediaDataURL(ctx context.Context, mediaID string) (string, error) {
	if c.accessToken == "" {
		return "", fmt.Errorf("whatsapp: media fetch needs an access token")
	}
	mreq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.graphBase+"/"+mediaID, nil)
	if err != nil {
		return "", err
	}
	mreq.Header.Set("Authorization", "Bearer "+c.accessToken)
	mresp, err := c.client.Do(mreq)
	if err != nil {
		return "", err
	}
	var meta struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
	}
	if err := func() error {
		defer mresp.Body.Close()
		if mresp.StatusCode/100 != 2 {
			return fmt.Errorf("whatsapp: media lookup status %d", mresp.StatusCode)
		}
		return json.NewDecoder(io.LimitReader(mresp.Body, maxBody)).Decode(&meta)
	}(); err != nil {
		return "", err
	}
	if meta.URL == "" {
		return "", fmt.Errorf("whatsapp: media has no download URL")
	}

	dreq, err := http.NewRequestWithContext(ctx, http.MethodGet, meta.URL, nil)
	if err != nil {
		return "", err
	}
	dreq.Header.Set("Authorization", "Bearer "+c.accessToken)
	dresp, err := c.client.Do(dreq)
	if err != nil {
		return "", err
	}
	defer dresp.Body.Close()
	if dresp.StatusCode/100 != 2 {
		return "", fmt.Errorf("whatsapp: media download status %d", dresp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(dresp.Body, waMediaMaxRaw+1))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("whatsapp: empty media")
	}
	if len(data) > waMediaMaxRaw {
		return "", fmt.Errorf("whatsapp: media exceeds %d bytes", waMediaMaxRaw)
	}
	mime := meta.MimeType
	if i := strings.IndexByte(mime, ';'); i >= 0 { // "audio/ogg; codecs=opus" → "audio/ogg"
		mime = strings.TrimSpace(mime[:i])
	}
	if mime == "" {
		mime = "audio/ogg"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// verify checks X-Hub-Signature-256: sha256=<hex HMAC-SHA256(appSecret, body)>,
// constant-time. An empty app secret fails closed.
