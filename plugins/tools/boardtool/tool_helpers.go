// SPDX-License-Identifier: MIT

package boardtool

// Provenance: tool_helpers.go: actor-identity + view-format + result helpers split
//             off from tool.go during the Day 211 god-file refactor (#128). Public
//             API unchanged.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

func applyActorIdentity(ctx context.Context, in *input) toolapi.Result {
	actor := strings.TrimSpace(toolapi.AgentFromContext(ctx))
	if actor == "" {
		return toolapi.Result{}
	}
	switch in.Op {
	case "post", "send", "reply", "broadcast", "help", "ack":
		from := strings.TrimSpace(in.From)
		if from == "" {
			in.From = actor
			return toolapi.Result{}
		}
		if from != actor {
			return errResult("acting agent " + actor + " cannot send board messages as " + from)
		}
	case "inbox":
		if strings.TrimSpace(in.To) == "" {
			in.To = actor
		}
	}
	return toolapi.Result{}
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
	if m.Help {
		v["help"] = true
	}
	if len(m.AckedBy) > 0 {
		v["acked_by"] = append([]string(nil), m.AckedBy...)
	}
	if m.TSMS > 0 {
		v["at"] = time.UnixMilli(m.TSMS).Format(time.RFC3339)
	}
	return v
}

func okJSON(v any) toolapi.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return toolapi.Result{Output: string(enc)}
}

func errResult(msg string) toolapi.Result {
	return toolapi.Result{Output: "board: " + msg, IsError: true}
}
