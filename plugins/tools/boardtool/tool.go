// SPDX-License-Identifier: MIT

package boardtool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/board"
)

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "board",
		Description: "A shared message board every agent on this daemon can use to coordinate: " +
			"op=post leaves a message on a topic; op=read returns recent messages (optionally for " +
			"one topic); op=topics lists the active topics. Direct agent-to-agent messaging: " +
			"op=send addresses a message to a named agent (to = its agent slug; returns the message id) " +
			"— it journals board.dm.<slug>, so a standing order can wake that agent; op=inbox lists what " +
			"is waiting for an agent (unanswered first); op=reply answers a message by id; op=replies " +
			"reads the answers to a message you sent. The board is shared and persistent.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "op":    {"type":"string", "enum":["post","read","topics","send","inbox","reply","replies"], "description":"What to do. Optional — if omitted it is inferred: text+to → send, text+id → reply, text (alone/with topic) → post, otherwise read."},
    "topic": {"type":"string", "description":"The topic to post under, or to filter reads by (a short label). For op=send: optional, defaults to \"dm\"."},
    "text":  {"type":"string", "description":"For op=post/send/reply: the message body."},
    "from":  {"type":"string", "description":"Who is posting/sending/replying — use YOUR agent slug so replies can find you."},
    "to":    {"type":"string", "description":"For op=send: the recipient agent's slug. For op=inbox: whose inbox to read (your slug)."},
    "id":    {"type":"string", "description":"For op=reply: the message id being answered. For op=replies: the message id whose answers to read."},
    "all":   {"type":"boolean", "description":"For op=inbox: include already-answered messages too (default false = only what's waiting)."},
    "limit": {"type":"integer", "description":"For op=read/inbox/replies: max messages (default 20, max 100)."}
  }
}`),
	}
}

type input struct {
	Op    string `json:"op"`
	Topic string `json:"topic"`
	Text  string `json:"text"`
	From  string `json:"from"`
	To    string `json:"to"`
	ID    string `json:"id"`
	All   bool   `json:"all"`
	Limit int    `json:"limit"`
}

// clampLimit applies the shared read-limit bounds.
func clampLimit(n int) int {
	if n <= 0 {
		return DefaultReadLimit
	}
	if n > MaxReadLimit {
		return MaxReadLimit
	}
	return n
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("board: parse input: %w", err)
	}
	st, nowFn, notify := t.current()
	if st == nil {
		return errResult("the board is not available on this daemon"), nil
	}

	// Infer a missing op from the supplied fields (M844): a workflow board node (or
	// an agent) that passes {topic, text} without an explicit op should post, not
	// fail. Keeps the common write ergonomic while leaving explicit ops untouched.
	in.Op = strings.ToLower(strings.TrimSpace(in.Op))
	if in.Op == "" {
		switch {
		case strings.TrimSpace(in.Text) != "" && strings.TrimSpace(in.To) != "":
			in.Op = "send"
		case strings.TrimSpace(in.Text) != "" && strings.TrimSpace(in.ID) != "":
			in.Op = "reply"
		case strings.TrimSpace(in.Text) != "":
			in.Op = "post"
		default:
			in.Op = "read"
		}
	}

	switch in.Op {
	case "post":
		if strings.TrimSpace(in.Topic) == "" {
			return errResult(`op=post needs a "topic"`), nil
		}
		if strings.TrimSpace(in.Text) == "" {
			return errResult(`op=post needs "text"`), nil
		}
		m, err := st.Post(in.Topic, in.From, in.Text, nowFn())
		if err != nil {
			return errResult(err.Error()), nil
		}
		// Journal the post so standing orders can react to it (M656). corr ties
		// the board.posted event to the run that posted (CorrelationFromContext).
		if notify != nil {
			notify(m, agent.CorrelationFromContext(ctx))
		}
		return okJSON(map[string]any{"posted": msgView(m)}), nil

	case "send":
		if strings.TrimSpace(in.To) == "" {
			return errResult(`op=send needs "to" (the recipient agent's slug)`), nil
		}
		if strings.TrimSpace(in.Text) == "" {
			return errResult(`op=send needs "text"`), nil
		}
		topic := strings.TrimSpace(in.Topic)
		if topic == "" {
			topic = "dm"
		}
		m, err := st.Send(board.Message{Topic: topic, From: in.From, To: in.To, Text: in.Text}, nowFn())
		if err != nil {
			return errResult(err.Error()), nil
		}
		if notify != nil {
			notify(m, agent.CorrelationFromContext(ctx))
		}
		return okJSON(map[string]any{"sent": msgView(m),
			"hint": "the recipient answers with op=reply id=" + m.ID + "; check op=replies id=" + m.ID}), nil

	case "inbox":
		if strings.TrimSpace(in.To) == "" {
			return errResult(`op=inbox needs "to" (whose inbox — your agent slug)`), nil
		}
		msgs := st.Inbox(in.To, clampLimit(in.Limit), in.All)
		views := make([]map[string]any, 0, len(msgs))
		for _, m := range msgs {
			views = append(views, msgView(m))
		}
		return okJSON(map[string]any{"to": in.To, "count": len(views), "waiting": views}), nil

	case "reply":
		if strings.TrimSpace(in.ID) == "" {
			return errResult(`op=reply needs "id" (the message being answered)`), nil
		}
		if strings.TrimSpace(in.Text) == "" {
			return errResult(`op=reply needs "text"`), nil
		}
		orig, ok := st.Get(strings.TrimSpace(in.ID))
		if !ok {
			return errResult("no message with id " + in.ID), nil
		}
		// The reply goes back to the asker: addressed to orig.From, same topic,
		// linked via ReplyTo so op=replies finds it.
		m, err := st.Send(board.Message{
			Topic: orig.Topic, From: in.From, To: orig.From, ReplyTo: orig.ID, Text: in.Text,
		}, nowFn())
		if err != nil {
			return errResult(err.Error()), nil
		}
		if notify != nil {
			notify(m, agent.CorrelationFromContext(ctx))
		}
		return okJSON(map[string]any{"replied": msgView(m)}), nil

	case "replies":
		if strings.TrimSpace(in.ID) == "" {
			return errResult(`op=replies needs "id" (your sent message's id)`), nil
		}
		msgs := st.Replies(strings.TrimSpace(in.ID), clampLimit(in.Limit))
		views := make([]map[string]any, 0, len(msgs))
		for _, m := range msgs {
			views = append(views, msgView(m))
		}
		return okJSON(map[string]any{"id": in.ID, "count": len(views), "replies": views}), nil

	case "read":
		msgs := st.Read(in.Topic, clampLimit(in.Limit))
		views := make([]map[string]any, 0, len(msgs))
		for _, m := range msgs {
			views = append(views, msgView(m))
		}
		out := map[string]any{"count": len(views), "messages": views}
		if strings.TrimSpace(in.Topic) != "" {
			out["topic"] = in.Topic
		}
		return okJSON(out), nil

	case "topics":
		return okJSON(map[string]any{"topics": st.Topics()}), nil

	case "":
		return errResult("op required (post|read|topics|send|inbox|reply|replies)"), nil
	default:
		return errResult("unknown op " + in.Op + " (post|read|topics|send|inbox|reply|replies)"), nil
	}
}

func msgView(m board.Message) map[string]any {
	v := map[string]any{"topic": m.Topic, "text": m.Text}
	if m.ID != "" {
		v["id"] = m.ID
	}
	if m.From != "" {
		v["from"] = m.From
	}
	if m.To != "" {
		v["to"] = m.To
	}
	if m.ReplyTo != "" {
		v["reply_to"] = m.ReplyTo
	}
	if m.TSMS > 0 {
		v["at"] = time.UnixMilli(m.TSMS).Format(time.RFC3339)
	}
	return v
}

func okJSON(v any) agent.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "board: " + msg, IsError: true}
}
