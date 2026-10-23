// SPDX-License-Identifier: MIT

// Package boardapi is the contract the message board and the tools that read
// it agree on.
//
// The board's message row is a contract between the module that owns the board
// and every consumer that shows a post — the board tool (a plugin), the
// overseer tool (a plugin), the control plane, the Web UI. A plugin may only
// reach the contract and the platform layers, so a row a plugin has to name
// cannot live in a module's implementation package.
//
// The rules of the road:
//
//   - Rows only. No store, no I/O.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
//
// The board's own package re-exports the row under its name, so the module,
// the host and the tools all speak the same type.
package boardapi

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

// Everyone is the recipient that means "every agent on the board". A message
// addressed to it lands in every inbox but the sender's.
const Everyone = "*"

// Store is the board-store port: the surface the board tool drives. It is a
// port and not the module's store because the tool is a PLUGIN, and a plugin
// may reach the contract and the platform layers and nothing else — so the
// store a plugin has to name cannot live in a module's implementation
// package. The board module's *board.Store implements it.
type Store interface {
	// Post appends a message to a topic.
	Post(topic, from, text string, nowMS int64) (Message, error)
	// Send appends a message (already addressed).
	Send(m Message, nowMS int64) (Message, error)
	// Broadcast appends a message to everyone.
	Broadcast(from, text string, nowMS int64) (Message, error)
	// HelpRequest appends a help request (optionally addressed).
	HelpRequest(from, to, text string, nowMS int64) (Message, error)
	// OpenHelp returns the still-unanswered help messages.
	OpenHelp(limit int) []Message
	// Get returns one message by id.
	Get(id string) (Message, bool)
	// Ack marks a message as read by one reader.
	Ack(id, by string) (Message, bool, error)
	// Inbox returns the messages addressed to one agent.
	Inbox(to string, limit int, includeAnswered bool) []Message
	// Replies returns the answers to one message.
	Replies(id string, limit int) []Message
	// Read returns the messages on one topic.
	Read(topic string, limit int) []Message
	// Topics returns the message count per topic.
	Topics() map[string]int
}
