// SPDX-License-Identifier: MIT

package whatsappgw

// Gateway JSON parsers: parseWAHA + parseEvolution. Carved out of
// whatsappgw.go during the Day 196 god-file split so the main
// file can stay focused on types + lifecycle + inbound handling
// and the send file can stay focused on outbound + emit helpers.
// Public API unchanged.

import (
	"encoding/json"
	"strings"
)

func parseWAHA(body []byte) []inbound {
	var w struct {
		Event   string `json:"event"`
		Payload struct {
			From   string `json:"from"`
			Body   string `json:"body"`
			ID     string `json:"id"`
			FromMe bool   `json:"fromMe"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return nil
	}
	if w.Event != "" && w.Event != "message" && w.Event != "message.any" {
		return nil
	}
	if w.Payload.FromMe || w.Payload.From == "" {
		return nil
	}
	return []inbound{{from: bareNumber(w.Payload.From), text: w.Payload.Body, id: w.Payload.ID}}
}

// parseEvolution reads an Evolution "messages.upsert" webhook:
// {event, data:{key:{remoteJid, id, fromMe}, message:{conversation | extendedTextMessage.text}}}.
func parseEvolution(body []byte) []inbound {
	var e struct {
		Event string `json:"event"`
		Data  struct {
			Key struct {
				RemoteJid string `json:"remoteJid"`
				ID        string `json:"id"`
				FromMe    bool   `json:"fromMe"`
			} `json:"key"`
			Message struct {
				Conversation        string `json:"conversation"`
				ExtendedTextMessage struct {
					Text string `json:"text"`
				} `json:"extendedTextMessage"`
			} `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return nil
	}
	if e.Data.Key.FromMe || e.Data.Key.RemoteJid == "" {
		return nil
	}
	text := e.Data.Message.Conversation
	if text == "" {
		text = e.Data.Message.ExtendedTextMessage.Text
	}
	return []inbound{{from: bareNumber(e.Data.Key.RemoteJid), text: text, id: e.Data.Key.ID}}
}

// bareNumber strips a WhatsApp jid suffix ("@c.us", "@s.whatsapp.net") to the
// bare number, the stable allowlist + reply key.
func bareNumber(jid string) string {
	if i := strings.IndexByte(jid, '@'); i >= 0 {
		return jid[:i]
	}
	return jid
}

// wahaChatID ensures a WAHA chatId has the "@c.us" suffix.
func wahaChatID(target string) string {
	if strings.Contains(target, "@") {
		return target
	}
	return target + "@c.us"
}

