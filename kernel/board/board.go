// SPDX-License-Identifier: MIT

// Package board is the in-process shared message board: a persistent,
// topic-addressed space every agent — the lead, its sub-agents, scheduled and
// standing-order agents, and the continuous loops — can post to and read from,
// so they can coordinate and talk to each other (M647).
//
// It is the common ground that complements memory (shared durable FACTS) and
// world (shared ENTITIES): the board carries shared MESSAGES — "I found X",
// "needs follow-up", a note an agent leaves for its next cycle or for a peer.
// One store, shared across every run on the daemon; survives restarts.
//
// Like kernel/cadence and kernel/standing, this is the pure store; the agent-
// facing `board` tool (plugins/tools/boardtool) wraps it, and the control plane
// reads it to surface the conversation in the Web UI. The store is a plain
// JSON file so a reader can Open it fresh and see the latest committed writes
// (writes are atomic: tmp + rename).
package board

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/ulid"
)

// MaxMessages bounds the on-disk board so it can't grow without limit; the
// oldest are dropped first.
const MaxMessages = 1000

// Message is one board post. Addressed messaging (M788): To names the
// recipient agent (a roster slug or any agreed name) for direct agent-to-agent
// messages; ReplyTo links an answer back to the message it answers; ID makes
// both possible. Plain topic posts (M647) simply leave them empty.
type Message struct {
	ID      string `json:"id,omitempty"`
	Topic   string `json:"topic"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	ReplyTo string `json:"reply_to,omitempty"`
	Text    string `json:"text"`
	// Help marks a message as a request for assistance (M849): it stays in
	// recipients' inboxes until any agent answers it, and OpenHelp surfaces the
	// still-unanswered ones so an overseer or peer can pick them up. A help
	// message is journaled under board.help[.<to>] so a standing order can wake a
	// responder.
	Help bool  `json:"help,omitempty"`
	TSMS int64 `json:"ts_ms"`
	// AckedBy lists the readers that explicitly acknowledged this message
	// (M937 mailbox): an acked message leaves that reader's unanswered Inbox
	// without needing a reply. Per-reader, so a broadcast acked by one agent
	// still shows for every other recipient.
	AckedBy []string `json:"acked_by,omitempty"`
}

// Everyone is the wildcard recipient for a broadcast (M849): a message To this
// value lands in every agent's Inbox except the sender's. It is not a real agent
// slug — Inbox special-cases it.
const Everyone = "*"

// Store is the persistent, concurrency-safe message board. Many agents (and
// their goroutines) post concurrently, so every access is mutex-guarded.
type Store struct {
	mu   sync.Mutex
	path string
	msgs []Message
}

// Open loads (or creates) the board under dir/board.json.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("board: mkdir: %w", err)
	}
	s := &Store{path: filepath.Join(dir, "board.json")}
	b, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(b, &s.msgs)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("board: read: %w", err)
	}
	return s, nil
}

// Post appends a topic message and persists. Kept as the M647 surface; it is
// Send with no recipient.
func (s *Store) Post(topic, from, text string, nowMS int64) (Message, error) {
	return s.Send(Message{Topic: topic, From: from, Text: text}, nowMS)
}

// Send appends a message — addressed (To set) or plain — assigning its ID and
// timestamp, and persists, dropping the oldest past MaxMessages (M788).
func (s *Store) Send(m Message, nowMS int64) (Message, error) {
	m.ID = ulid.New()
	m.Topic = strings.TrimSpace(m.Topic)
	m.From = strings.TrimSpace(m.From)
	m.To = strings.TrimSpace(m.To)
	m.ReplyTo = strings.TrimSpace(m.ReplyTo)
	m.TSMS = nowMS
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, m)
	if len(s.msgs) > MaxMessages {
		s.msgs = s.msgs[len(s.msgs)-MaxMessages:]
	}
	return m, s.save()
}

// Broadcast appends a message addressed to Everyone (M849): it lands in every
// agent's Inbox except the sender's. A plain announcement — "I shipped X",
// "heads-up, deploying" — that any agent sees when it checks its inbox.
func (s *Store) Broadcast(from, text string, nowMS int64) (Message, error) {
	return s.Send(Message{Topic: "broadcast", From: from, To: Everyone, Text: text}, nowMS)
}

// HelpRequest appends a request for assistance (M849). If to is empty it is
// broadcast to Everyone (any agent may answer); if to names an agent it is a
// directed ask. Either way it is flagged Help, so it stays in the inbox until
// answered and OpenHelp surfaces it.
func (s *Store) HelpRequest(from, to, text string, nowMS int64) (Message, error) {
	to = strings.TrimSpace(to)
	if to == "" {
		to = Everyone
	}
	return s.Send(Message{Topic: "help", From: from, To: to, Text: text, Help: true}, nowMS)
}

// OpenHelp returns up to limit still-UNANSWERED help requests, newest first —
// the "who needs help right now" view for an overseer agent or the Web UI. A
// help request is open until any message replies to it (ReplyTo == its id).
func (s *Store) OpenHelp(limit int) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	answered := map[string]bool{}
	for _, m := range s.msgs {
		if m.ReplyTo != "" {
			answered[m.ReplyTo] = true
		}
	}
	out := make([]Message, 0, 8)
	for _, m := range s.msgs {
		if m.Help && !answered[m.ID] {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSMS > out[j].TSMS })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Get returns one message by id.
func (s *Store) Get(id string) (Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.msgs {
		if m.ID == id {
			return m, true
		}
	}
	return Message{}, false
}

// Ack marks the message id as read by `by` (M937): it leaves by's unanswered
// Inbox without a reply being written. Per-reader and idempotent. Reports
// whether the id exists; a blank `by` acks nothing (the caller validates).
func (s *Store) Ack(id, by string) (Message, bool, error) {
	by = strings.TrimSpace(by)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.msgs {
		if s.msgs[i].ID != id {
			continue
		}
		if by == "" || ackedBy(s.msgs[i], strings.ToLower(by)) {
			return s.msgs[i], true, nil
		}
		s.msgs[i].AckedBy = append(s.msgs[i].AckedBy, by)
		return s.msgs[i], true, s.save()
	}
	return Message{}, false, nil
}

// ackedBy reports whether reader (already lowercased) acknowledged m.
func ackedBy(m Message, reader string) bool {
	for _, a := range m.AckedBy {
		if strings.ToLower(strings.TrimSpace(a)) == reader {
			return true
		}
	}
	return false
}

// Inbox returns up to limit messages ADDRESSED to `to` (case-insensitive),
// newest first. With includeAnswered=false (the usual call), direct messages
// and help requests that already have a reply are dropped. Normal broadcasts
// are per-reader: a reply only clears the broadcast for the replying reader,
// while Ack clears it for the explicit reader.
func (s *Store) Inbox(to string, limit int, includeAnswered bool) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	to = strings.ToLower(strings.TrimSpace(to))
	answered := map[string]bool{}
	repliedBy := map[string]map[string]bool{}
	if !includeAnswered {
		for _, m := range s.msgs {
			if m.ReplyTo != "" {
				answered[m.ReplyTo] = true
				from := strings.ToLower(strings.TrimSpace(m.From))
				if from != "" {
					if repliedBy[m.ReplyTo] == nil {
						repliedBy[m.ReplyTo] = map[string]bool{}
					}
					repliedBy[m.ReplyTo][from] = true
				}
			}
		}
	}
	out := make([]Message, 0, 8)
	for _, m := range s.msgs {
		// A message is "for me" if it is addressed to my slug, OR it is a broadcast
		// (To == Everyone) that I didn't send (M849).
		mTo := strings.ToLower(strings.TrimSpace(m.To))
		directed := mTo != "" && mTo == to
		broadcast := m.To == Everyone && strings.ToLower(strings.TrimSpace(m.From)) != to
		if !directed && !broadcast {
			continue
		}
		if !includeAnswered {
			if ackedBy(m, to) {
				continue
			}
			if directed && answered[m.ID] {
				continue
			}
			if broadcast {
				if m.Help && answered[m.ID] {
					continue
				}
				if repliedBy[m.ID][to] {
					continue
				}
			}
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSMS > out[j].TSMS })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Replies returns up to limit replies to the message id, OLDEST first
// (conversation order) — what the asker reads back.
func (s *Store) Replies(id string, limit int) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, 0, 4)
	for _, m := range s.msgs {
		if m.ReplyTo == id {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSMS < out[j].TSMS })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Read returns up to limit messages, newest first, optionally filtered to a
// topic (case-insensitive exact match).
func (s *Store) Read(topic string, limit int) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	topic = strings.ToLower(strings.TrimSpace(topic))
	out := make([]Message, 0, len(s.msgs))
	for _, m := range s.msgs {
		if topic == "" || strings.ToLower(m.Topic) == topic {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSMS > out[j].TSMS })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Topics returns the distinct topics on the board with their message counts.
func (s *Store) Topics() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := map[string]int{}
	for _, m := range s.msgs {
		counts[m.Topic]++
	}
	return counts
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.msgs, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
