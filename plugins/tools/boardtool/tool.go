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
			"one topic); op=topics lists the active topics. Use it to talk to other agents, hand off " +
			"findings, or leave a note for your next cycle. The board is shared and persistent.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":    {"type":"string", "enum":["post","read","topics"]},
    "topic": {"type":"string", "description":"The topic to post under, or to filter reads by (a short label)."},
    "text":  {"type":"string", "description":"For op=post: the message body."},
    "from":  {"type":"string", "description":"For op=post (optional): who is posting (e.g. a role like \"researcher\")."},
    "limit": {"type":"integer", "description":"For op=read: max messages (default 20, max 100)."}
  }
}`),
	}
}

type input struct {
	Op    string `json:"op"`
	Topic string `json:"topic"`
	Text  string `json:"text"`
	From  string `json:"from"`
	Limit int    `json:"limit"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(_ context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("board: parse input: %w", err)
	}
	st, nowFn := t.current()
	if st == nil {
		return errResult("the board is not available on this daemon"), nil
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
		return okJSON(map[string]any{"posted": msgView(m)}), nil

	case "read":
		limit := in.Limit
		if limit <= 0 {
			limit = DefaultReadLimit
		}
		if limit > MaxReadLimit {
			limit = MaxReadLimit
		}
		msgs := st.Read(in.Topic, limit)
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
		return errResult("op required (post|read|topics)"), nil
	default:
		return errResult("unknown op " + in.Op + " (post|read|topics)"), nil
	}
}

func msgView(m board.Message) map[string]any {
	v := map[string]any{"topic": m.Topic, "text": m.Text}
	if m.From != "" {
		v["from"] = m.From
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
