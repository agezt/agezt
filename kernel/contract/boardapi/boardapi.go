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
