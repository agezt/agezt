// SPDX-License-Identifier: MIT

package bedrock

// DeepSeek-R1-on-Bedrock body shape (M328). DeepSeek's reasoning models
// (`deepseek.r1-*`) are text-to-text on Bedrock's InvokeModel: like the
// Meta-Llama adapter there's no structured messages array — the caller
// renders the conversation into a single `prompt` string using DeepSeek's
// chat template, and the model continues from an opened thinking block.
//
//	{
//	  "prompt":      "<｜begin▁of▁sentence｜><｜User｜>...<｜Assistant｜><think>\n",
//	  "max_tokens":  N,
//	  "temperature": ...
//	}
//
// Response (InvokeModel text completion — verified against AWS docs):
//
//	{
//	  "choices": [{"text": string, "stop_reason": "stop" | "length"}]
//	}
//
// DeepSeek-R1 is a REASONING model. Because the prompt ends with an open
// `<think>`, the returned `text` begins with the chain of thought, then a
// `</think>` marker, then the answer. This adapter splits on `</think>`:
// the thinking becomes ReasoningContent (M317 pipeline — surfaced to pulse,
// ACP, and the OpenAI-compatible API), the rest the answer. Token counts
// are not in this body; the Bedrock response-header overlay in Complete
// (M327) supplies them.
//
// **No tool use** — chat-only, like the other non-Anthropic adapters.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)

// DeepSeek chat-template special tokens. These use full-width vertical bars
// (U+FF5C) and the SentencePiece underline (U+2581); they must match the
// model's tokenizer exactly, so they are pinned here as named constants.
const (
	dsBOS       = "<｜begin▁of▁sentence｜>"
	dsEOS       = "<｜end▁of▁sentence｜>"
	dsUser      = "<｜User｜>"
	dsAssistant = "<｜Assistant｜>"
	// dsThinkOpen is appended after the trailing assistant tag so R1 starts
	// in its thinking block (AWS's documented prompt format).
	dsThinkOpen  = "<think>\n"
	dsThinkClose = "</think>"
)

type deepseekBedrockRequest struct {
	Prompt    string `json:"prompt"`
	MaxTokens int    `json:"max_tokens"`
}

type deepseekBedrockResponse struct {
	Choices []struct {
		Text       string `json:"text"`
		StopReason string `json:"stop_reason"`
	} `json:"choices"`
}

// deepseekR1Template renders system + messages into DeepSeek-R1's chat prompt:
// BOS, the system prompt as a bare prefix, then alternating <｜User｜> /
// <｜Assistant｜> turns, ending with an open assistant tag + <think> so the
// model produces its reasoning then the answer.
func deepseekR1Template(system string, msgs []agent.Message) string {
	var sb strings.Builder
	sb.WriteString(dsBOS)
	if s := strings.TrimSpace(system); s != "" {
		sb.WriteString(s)
	}
	for _, m := range msgs {
		switch m.Role {
		case agent.RoleAssistant:
			sb.WriteString(dsAssistant)
			sb.WriteString(m.Content)
			sb.WriteString(dsEOS)
		default:
			// user, tool results, and any per-message system role fold into a
			// user turn — DeepSeek's template has only user/assistant turns.
			sb.WriteString(dsUser)
			sb.WriteString(m.Content)
		}
	}
	sb.WriteString(dsAssistant)
	sb.WriteString(dsThinkOpen)
	return sb.String()
}

func encodeDeepSeekOnBedrockRequest(system string, msgs []agent.Message, maxTok int) ([]byte, error) {
	if len(msgs) == 0 {
		return nil, errors.New("bedrock-deepseek: at least one message required")
	}
	out := deepseekBedrockRequest{
		Prompt:    deepseekR1Template(system, msgs),
		MaxTokens: maxTok,
	}
	return json.Marshal(out)
}

// decodeDeepSeekOnBedrockResponse splits the completion text into the model's
// reasoning (before </think>) and answer (after). Usage is left zero — the
// InvokeModel text-completion body carries no token counts; Complete's
// response-header overlay (M327) fills them.
func decodeDeepSeekOnBedrockResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var wire deepseekBedrockResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("bedrock-deepseek: parse response: %w", err)
	}
	if len(wire.Choices) == 0 {
		return nil, errors.New("bedrock-deepseek: response has no choices")
	}
	ch := wire.Choices[0]
	if strings.TrimSpace(ch.Text) == "" {
		return nil, errors.New("bedrock-deepseek: response text empty")
	}

	var reasoning, answer string
	if before, after, found := strings.Cut(ch.Text, dsThinkClose); found {
		reasoning = strings.TrimSpace(before)
		answer = strings.TrimSpace(after)
	} else {
		// No closing tag — either the run was truncated mid-thought (stop_reason
		// "length") or the model skipped explicit thinking. Surface the text as
		// the answer so the caller never gets an empty response.
		answer = strings.TrimSpace(ch.Text)
	}

	stop := agent.StopEndTurn
	if strings.EqualFold(ch.StopReason, "length") {
		stop = agent.StopMaxTokens
	}
	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: answer,
		},
		ReasoningContent: reasoning,
		StopReason:       stop,
		Usage:            agent.Usage{Model: model},
	}, nil
}

// isDeepSeekModel reports whether the model id maps to the DeepSeek-on-Bedrock
// body shape (M328). Matches `deepseek.*` and regional cross-inference profiles
// (`us.deepseek.r1-v1:0`, `eu.deepseek.*`).
func isDeepSeekModel(id string) bool {
	if strings.HasPrefix(id, "deepseek.") {
		return true
	}
	return strings.Contains(id, ".deepseek.")
}
