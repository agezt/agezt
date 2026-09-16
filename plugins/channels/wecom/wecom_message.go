// SPDX-License-Identifier: MIT

// wecom_message.go owns the request-handling side of the WeCom
// channel: handleInbound (HTTP → parsed inbound) and dispatch
// (parsed inbound → kernel bus + sender reply). Types +
// lifecycle (New / Start / Handler / emitInbound / seenBefore)
// live in wecom.go; crypto helpers live in wecom_crypto.go.
package wecom

import (
	"context"
	"crypto/subtle"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/ulid"
)

func (c *Channel) handleInbound(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sig, timestamp, nonce := q.Get("msg_signature"), q.Get("timestamp"), q.Get("nonce")

	// Fail closed on an unconfigured callback token: `signature` below is a plain
	// SHA-1 over the sorted (token, timestamp, nonce, payload) tuple, so an empty
	// token makes the expected digest attacker-computable and the comparison
	// below a no-op. (Decryption is a second gate, not this one.) A missing
	// msg_signature is already rejected by that comparison once a token is set.
	if c.cfg.Token == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// GET: URL verification — decrypt echostr and echo the plaintext.
	if r.Method == http.MethodGet {
		echo := q.Get("echostr")
		if signature(c.cfg.Token, timestamp, nonce, echo) != sig {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		msg, _, err := c.decrypt(echo)
		if err != nil {
			http.Error(w, "bad echostr", http.StatusBadRequest)
			return
		}
		_, _ = w.Write(msg)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	var env struct {
		Encrypt string `xml:"Encrypt"`
	}
	if err := xml.Unmarshal(body, &env); err != nil || env.Encrypt == "" {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if signature(c.cfg.Token, timestamp, nonce, env.Encrypt) != sig {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	plain, receiveID, err := c.decrypt(env.Encrypt)
	if err != nil {
		http.Error(w, "decrypt failed", http.StatusBadRequest)
		return
	}
	// The trailing receive_id must equal our corp id (WXBizMsgCrypt spec) — guards
	// against payloads encrypted for a different corp being replayed at us.
	if c.cfg.CorpID != "" && subtle.ConstantTimeCompare([]byte(receiveID), []byte(c.cfg.CorpID)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK) // WeCom accepts an empty 200; reply goes via the API.
	if m, ok := parseMessage(plain); ok {
		c.dispatch(r.Context(), m)
	}
}

func (c *Channel) dispatch(ctx context.Context, m inbound) {
	if m.sender == "" || (strings.TrimSpace(m.text) == "" && m.mediaID == "") {
		return
	}
	if m.id != "" && c.seenBefore(m.id) {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "wecom",
		ChannelID:    m.sender,
		Sender:       m.sender,
		Text:         m.text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.cfg.Allowlist.Allows(m.sender)
	// Inbound media: fetch by media id (allowlisted senders only) so an image
	// reaches a vision model and a voice clip is transcribed.
	if allowed && m.mediaID != "" {
		if du := c.fetchMedia(ctx, m.mediaID, m.mediaType); du != "" {
			if m.mediaType == "audio" {
				msg.Audio = []string{du}
			} else {
				msg.Images = []string{du}
			}
		}
	}
	c.emitInbound(msg, corr, allowed)
	if !allowed || c.cfg.Handler == nil {
		return
	}
	rep, err := c.cfg.Handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	reply := rep.Text
	if reply == "" {
		return
	}
	_ = c.Send(ctx, channel.Outbound{ChannelID: m.sender, Text: reply, Priority: channel.PriorityNotify})
}
