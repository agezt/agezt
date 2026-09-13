// SPDX-License-Identifier: MIT

package discord

// Discord attachment + verify + outbound + helpers + emit: discordAttachMaxRaw
// + validDiscordAttachmentURL + fetchAttachmentDataURL + runAndFollowUp +
// verify + discordMaxChars + Send + followUp + followUpMedia + do +
// writeJSON + ephemeral + emitInbound + emitOutbound. Carved out of
// discord.go during the Day 179 god-file split so the main file can stay
// focused on Config + Channel + lifecycle + HTTP handlers.
// Public API unchanged.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
)
const discordAttachMaxRaw = 12 << 20

// validDiscordAttachmentURL accepts only https URLs whose host is a Discord CDN
// host (cdn.discordapp.com / media.discordapp.net, or any sub-domain of
// discordapp.com / discordapp.net). This bounds the server-side attachment fetch
// to Discord's own content network, so the URL field of a (signed) interaction
// can't be used to make the daemon fetch an arbitrary host. (H-001)
func validDiscordAttachmentURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("discord attachment: bad url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("discord attachment: non-https url rejected")
	}
	host := strings.ToLower(u.Hostname())
	ok := host == "cdn.discordapp.com" || host == "media.discordapp.net" ||
		strings.HasSuffix(host, ".discordapp.com") || strings.HasSuffix(host, ".discordapp.net")
	if !ok {
		return fmt.Errorf("discord attachment: host %q is not a Discord CDN host", host)
	}
	return nil
}

// fetchAttachmentDataURL downloads a Discord attachment (a public CDN url, no
// auth) and returns it as an inline data: URL using the attachment's reported
// content type (M249).
func (c *Channel) fetchAttachmentDataURL(ctx context.Context, att discordAttachment) (string, error) {
	// Defense-in-depth (H-001): the attachment URL arrives inside a signed Discord
	// interaction and should always be an https Discord-CDN URL — but this is a
	// server-side fetch driven by provider JSON, so validate the scheme + host
	// before dialing rather than trusting the field. Anything that isn't https on
	// a discordapp CDN host is refused, so a malformed/hostile URL can't turn this
	// into an SSRF probe of arbitrary hosts.
	check := c.attachURLOK
	if check == nil {
		check = validDiscordAttachmentURL
	}
	if err := check(att.URL); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, att.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("discord attachment download: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, discordAttachMaxRaw+1))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("discord attachment download: empty")
	}
	if len(data) > discordAttachMaxRaw {
		return "", fmt.Errorf("discord attachment exceeds %d bytes", discordAttachMaxRaw)
	}
	return "data:" + att.ContentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func (c *Channel) runAndFollowUp(ctx context.Context, in discordInteraction, msg channel.UnifiedMessage, corr string) {
	// Inbound image attachments (M249): fetch them now (after the fast ACK, so
	// the download never risks the 3s interaction deadline) and attach as data:
	// URLs so a vision model can see them. The allowlist was already enforced
	// before this runs.
	for _, att := range in.imageAttachments() {
		if du, err := c.fetchAttachmentDataURL(ctx, att); err == nil && du != "" {
			msg.Images = append(msg.Images, du)
		}
	}
	// Inbound voice messages / audio clips: fetch as data: URLs so the ambient
	// STT path (when AGEZT_STT_* is set) transcribes them into the agent's intent.
	for _, att := range in.audioAttachments() {
		if du, err := c.fetchAttachmentDataURL(ctx, att); err == nil && du != "" {
			msg.Audio = append(msg.Audio, du)
		}
	}
	rep, err := c.handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	reply := rep.Text
	if reply == "" && len(rep.Attachments) == 0 {
		reply = "(no output)"
	}
	_ = c.followUp(ctx, in.Token, in.ChannelID, reply, rep.Attachments, corr)
}

// verify checks Discord's Ed25519 request signature over (timestamp || body),
// with a timestamp freshness window for replay protection. A missing/invalid
// public key fails closed.
func (c *Channel) verify(ts, sigHex string, body []byte) bool {
	if len(c.pubKey) != ed25519.PublicKeySize || ts == "" || sigHex == "" {
		return false
	}
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	delta := c.now().Unix() - n
	if delta < 0 {
		delta = -delta
	}
	// Integer-seconds comparison: time.Duration(delta)*time.Second overflows int64
	// nanoseconds for a far-off timestamp and could wrap negative (passing the
	// `> window` check). Signed input makes this unreachable today; a freshness
	// backstop shouldn't rely on that.
	if delta > int64(signatureWindow/time.Second) {
		return false
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	msg := make([]byte, 0, len(ts)+len(body))
	msg = append(msg, ts...)
	msg = append(msg, body...)
	return ed25519.Verify(c.pubKey, msg, sig)
}

// --- outbound -------------------------------------------------------------

// Send implements channel.Channel: post a message to a channel via the bot token
// (Pulse→Discord sink and out-of-band senders). Distinct from the interaction
// follow-up path, which authenticates with the interaction token, not the bot.
// discordMaxChars is Discord's per-message content limit (2000 characters). A
// longer message is rejected, so a long answer is split into sequential
// messages rather than lost (M234).
const discordMaxChars = 2000

func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	// Discord rejects an empty message; no-op rather than fail (M236).
	if strings.TrimSpace(out.Text) == "" {
		return nil
	}
	for _, chunk := range channel.SplitText(out.Text, discordMaxChars) {
		body, _ := json.Marshal(map[string]any{"content": chunk})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/channels/"+out.ChannelID+"/messages", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bot "+c.token)
		if err := c.do(req); err != nil {
			return err
		}
	}
	c.emitOutbound(out, "")
	return nil
}

// followUp delivers an interaction's answer via the follow-up webhook
// (webhooks/{app}/{token}); the token in the URL authenticates, so no bot header.
// Like Send, it chunks past Discord's 2000-char limit (M234) — each POST to the
// follow-up webhook creates a new message — so a long slash-command answer is
// delivered in sequence rather than rejected and lost.
func (c *Channel) followUp(ctx context.Context, token, channelID, content string, atts []channel.Attachment, corr string) error {
	if strings.TrimSpace(content) == "" && len(atts) == 0 {
		return nil
	}
	url := c.base + "/webhooks/" + c.appID + "/" + token
	for _, chunk := range channel.SplitText(content, discordMaxChars) {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		body, _ := json.Marshal(map[string]any{"content": chunk})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if err := c.do(req); err != nil {
			return err
		}
	}
	// Outbound media: each attachment is a multipart follow-up message
	// (voice clip, image, file) — Discord renders voice/images inline.
	for _, att := range atts {
		if err := c.followUpMedia(ctx, url, att); err != nil {
			return err
		}
	}
	c.emitOutbound(channel.Outbound{ChannelID: channelID, Text: content, Priority: channel.PriorityNotify}, corr)
	return nil
}

// followUpMedia posts one attachment to the interaction follow-up webhook as
// multipart/form-data (payload_json + files[0]).
func (c *Channel) followUpMedia(ctx context.Context, url string, att channel.Attachment) error {
	if len(att.Data) == 0 {
		return nil
	}
	fn := att.Filename
	if fn == "" {
		fn = "file"
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("payload_json", `{"attachments":[{"id":0,"filename":`+strconv.Quote(fn)+`}]}`)
	part, err := mw.CreateFormFile("files[0]", fn)
	if err != nil {
		return err
	}
	if _, err := part.Write(att.Data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return c.do(req)
}

func (c *Channel) do(req *http.Request) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("discord: status %d", resp.StatusCode)
	}
	return nil
}

// --- helpers / journaling --------------------------------------------------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ephemeral builds an immediate, invoker-only message response.
func ephemeral(text string) map[string]any {
	return map[string]any{
		"type": responseMessage,
		"data": map[string]any{"content": text, "flags": flagEphemeral},
	}
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.bus == nil {
		return
	}
	_, _ = c.bus.Publish(event.Spec{
		Subject:       "channel.inbound.discord",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-discord",
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
		Subject:       "channel.outbound.discord",
		Kind:          event.KindChannelOutbound,
		Actor:         "channel-discord",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "discord",
			"channel_id":   out.ChannelID,
			"text":         out.Text,
			"priority":     string(out.Priority),
		},
	})
}
