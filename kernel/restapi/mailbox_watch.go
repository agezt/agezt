// SPDX-License-Identifier: MIT

package restapi

// Mailbox watch stream: boardPostedPayload type + mailWatchMatch +
// handleMailboxWatch + handleMailboxTopics. Carved out of
// mailbox.go during the Day 190 god-file split so the main file
// can stay focused on lifecycle setters + auth + view helpers and
// the handlers file can stay focused on the message CRUD handlers.
// Public API unchanged.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/httpserver"
)

type boardPostedPayload struct {
	ID    string `json:"id"`
	Topic string `json:"topic"`
	From  string `json:"from"`
	To    string `json:"to"`
	Help  bool   `json:"help"`
}

// mailWatchMatch reports whether a posted message is for this watcher: with a
// name, messages addressed to it (case-insensitive) plus broadcasts it didn't
// send — the live counterpart of Inbox; with a topic, that topic's posts; with
// neither, everything (a firehose tail).
func mailWatchMatch(name, topic string, p boardPostedPayload) bool {
	if name != "" {
		to := strings.ToLower(strings.TrimSpace(p.To))
		directed := to != "" && to == name
		broadcast := p.To == board.Everyone && strings.ToLower(strings.TrimSpace(p.From)) != name
		if !directed && !broadcast {
			return false
		}
	}
	if topic != "" && !strings.EqualFold(strings.TrimSpace(p.Topic), topic) {
		return false
	}
	return true
}

// handleMailboxWatch streams new mailbox messages as SSE `mail` frames the
// moment they land (M938) — the push counterpart of polling the inbox.
// ?name= watches one agent/app's mail (DMs + broadcasts), ?topic= one topic,
// neither tails everything. The mailbox is daemon-global, so this subscribes
// the primary bus regardless of any tenant header.
func (s *Server) handleMailboxWatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	st, ok := s.mailbox(w)
	if !ok {
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("name")))
	topic := strings.TrimSpace(r.URL.Query().Get("topic"))

	// Subscribe BEFORE the ready frame so no message can slip between them.
	sub, err := s.bus.Subscribe("board.>", 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "subscribe_error", err.Error())
		return
	}
	defer sub.Cancel()

	// StartSSE bounds concurrent watch streams per client (V-009/LD-8).
	sse, ok := httpserver.StartSSE(w, r)
	if !ok {
		return
	}
	defer sse.Close()

	send := func(eventName string, payload any) {
		_ = sse.WriteEvent(eventName, payload)
	}
	ready := map[string]any{}
	if name != "" {
		ready["name"] = name
	}
	if topic != "" {
		ready["topic"] = topic
	}
	send("ready", ready)

	keepalive := time.NewTicker(mailboxKeepalive)
	defer keepalive.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			_ = sse.Comment("keepalive")
		case ev, alive := <-sub.C:
			if !alive {
				return
			}
			if ev == nil || ev.Kind != event.KindBoardPosted || len(ev.Payload) == 0 {
				continue
			}
			var p boardPostedPayload
			if json.Unmarshal(ev.Payload, &p) != nil {
				continue
			}
			if !mailWatchMatch(name, topic, p) {
				continue
			}
			// The event carries metadata only; the body lives in the store. The
			// write committed before the publish, so the lookup is reliable — but
			// fall back to the metadata view if the message was already evicted.
			if m, found := st.Get(p.ID); found {
				send("mail", mailMsgView(m))
				continue
			}
			send("mail", map[string]any{
				"id": p.ID, "topic": p.Topic, "from": p.From, "to": p.To, "help": p.Help,
			})
		}
	}
}

// --- GET /api/v1/mailbox/topics ---

func (s *Server) handleMailboxTopics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	st, ok := s.mailbox(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": st.Topics()})
}

