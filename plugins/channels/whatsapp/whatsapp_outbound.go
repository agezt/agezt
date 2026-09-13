// SPDX-License-Identifier: MIT

package whatsapp

// WhatsApp outbound + webhook processing: verify + Send + send + sendMedia
// + uploadMedia + waWebhook + inboundMsg + messages + emitInbound +
// emitOutbound + sign + dedup + newDedup. Carved out of whatsapp.go
// during the Day 173 god-file split so the main file can stay focused
// on lifecycle + HTTP handlers + media fetch.
// Public API unchanged.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)
func (c *Channel) verify(sig string, body []byte) bool {
	if c.appSecret == "" || sig == "" {
		return false
	}
	got := strings.TrimPrefix(sig, "sha256=")
	want := sign(c.appSecret, body)
	return hmac.Equal([]byte(got), []byte(want))
}

// --- outbound -------------------------------------------------------------

// Send implements channel.Channel: send a WhatsApp text to out.ChannelID via the
// Graph API, splitting long text. Errors when credentials are missing.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	if strings.TrimSpace(out.Text) == "" && len(out.Attachments) == 0 {
		return nil
	}
	corr := "chan-" + ulid.New()
	return c.send(ctx, out, corr)
}

func (c *Channel) send(ctx context.Context, out channel.Outbound, corr string) error {
	if c.accessToken == "" || c.phoneID == "" {
		return fmt.Errorf("whatsapp: outbound not configured (set AccessToken + PhoneNumberID)")
	}
	endpoint := c.graphBase + "/" + c.phoneID + "/messages"
	for _, chunk := range channel.SplitText(out.Text, waMaxChars) {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"messaging_product": "whatsapp",
			"to":                out.ChannelID,
			"type":              "text",
			"text":              map[string]any{"body": chunk},
		})
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
		resp, err := c.client.Do(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("whatsapp: Graph API returned status %d", resp.StatusCode)
		}
	}
	for _, att := range out.Attachments {
		if err := c.sendMedia(ctx, out.ChannelID, att); err != nil {
			return err
		}
	}
	c.emitOutbound(out, corr)
	return nil
}

// sendMedia delivers one attachment via the WhatsApp Cloud API's two-step flow:
// upload the bytes to /{phoneID}/media (multipart) → POST a typed message
// referencing the returned media id.
func (c *Channel) sendMedia(ctx context.Context, to string, att channel.Attachment) error {
	if len(att.Data) == 0 {
		return nil
	}
	id, err := c.uploadMedia(ctx, att)
	if err != nil || id == "" {
		return err
	}
	mtype := "document"
	switch att.Kind {
	case "audio":
		mtype = "audio"
	case "image":
		mtype = "image"
	}
	media := map[string]any{"id": id}
	if mtype == "document" && att.Filename != "" {
		media["filename"] = att.Filename
	}
	payload, _ := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              mtype,
		mtype:               media,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphBase+"/"+c.phoneID+"/messages", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("whatsapp: send media returned status %d", resp.StatusCode)
	}
	return nil
}

// uploadMedia POSTs the attachment bytes to /{phoneID}/media and returns the
// media id.
func (c *Channel) uploadMedia(ctx context.Context, att channel.Attachment) (string, error) {
	fn := att.Filename
	if fn == "" {
		fn = "file"
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("messaging_product", "whatsapp")
	if att.MIME != "" {
		_ = mw.WriteField("type", att.MIME)
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, fn))
	if att.MIME != "" {
		h.Set("Content-Type", att.MIME)
	}
	fw, err := mw.CreatePart(h)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(att.Data); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphBase+"/"+c.phoneID+"/media", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("whatsapp: media upload returned status %d", resp.StatusCode)
	}
	var r struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &r)
	return r.ID, nil
}

// --- wire shapes ----------------------------------------------------------

type waWebhook struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Messages []struct {
					From string `json:"from"`
					ID   string `json:"id"`
					Type string `json:"type"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
					Audio struct {
						ID string `json:"id"`
					} `json:"audio"`
					Voice struct {
						ID string `json:"id"`
					} `json:"voice"`
					Image struct {
						ID      string `json:"id"`
						Caption string `json:"caption"`
					} `json:"image"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type inboundMsg struct {
	from, id, text string
	audioID        string // set for voice/audio messages (Cloud API media id)
	imageID        string // set for image messages (Cloud API media id)
}

// messages flattens the nested webhook envelope to the messages it carries.
// Text and voice/audio messages are kept (a voice note is fetched + transcribed
// downstream); other media types are ignored.
func (w waWebhook) messages() []inboundMsg {
	var out []inboundMsg
	for _, e := range w.Entry {
		for _, ch := range e.Changes {
			for _, m := range ch.Value.Messages {
				switch m.Type {
				case "text":
					out = append(out, inboundMsg{from: m.From, id: m.ID, text: m.Text.Body})
				case "audio", "voice":
					// WhatsApp Cloud API tags voice notes as type "audio" with
					// audio.voice=true; some shapes use a "voice" object. Accept both.
					id := m.Audio.ID
					if id == "" {
						id = m.Voice.ID
					}
					out = append(out, inboundMsg{from: m.From, id: m.ID, audioID: id})
				case "image":
					out = append(out, inboundMsg{from: m.From, id: m.ID, text: m.Image.Caption, imageID: m.Image.ID})
				}
			}
		}
	}
	return out
}

// --- events ---------------------------------------------------------------

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.whatsapp",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-whatsapp",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": msg.ChannelKind,
			"channel_id":   msg.ChannelID,
			"sender":       msg.Sender,
			"text":         msg.Text,
			"allowed":      allowed,
		},
	})
}

func (c *Channel) emitOutbound(out channel.Outbound, corr string) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.outbound.whatsapp",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-whatsapp",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_id": out.ChannelID,
			"text":       out.Text,
			"priority":   string(out.Priority),
		},
	})
}

// --- helpers --------------------------------------------------------------

// sign returns the hex HMAC-SHA256 of body under secret (Meta's
// X-Hub-Signature-256 scheme).
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// dedup is a small bounded set of recently-seen message ids (Meta retries
// deliveries). Two generations so eviction never forgets every id at once;
// memory bounded at 2×cap. Mirrors the webhook/sms channels.
type dedup struct {
	mu   sync.Mutex
	seen map[string]struct{}
	prev map[string]struct{}
	cap  int
}

func newDedup(capacity int) *dedup {
	return &dedup{seen: make(map[string]struct{}, capacity), cap: capacity}
}

func (d *dedup) seenBefore(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[key]; ok {
		return true
	}
	if _, ok := d.prev[key]; ok {
		return true
	}
	if len(d.seen) >= d.cap {
		d.prev = d.seen
		d.seen = make(map[string]struct{}, d.cap)
	}
	d.seen[key] = struct{}{}
	return false
}
