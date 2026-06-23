// SPDX-License-Identifier: MIT

package openai

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// decodeMessages pulls the messages out of an encoded request with content left
// as raw JSON, so a test can tell the string form from the parts-array form.
func decodeMessages(t *testing.T, body []byte) []struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	ToolCalls json.RawMessage `json:"tool_calls"`
} {
	t.Helper()
	var got struct {
		Messages []struct {
			Role      string          `json:"role"`
			Content   json.RawMessage `json:"content"`
			ToolCalls json.RawMessage `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	return got.Messages
}

// A user message carrying an image URL is encoded as OpenAI's multimodal
// content-parts array: a text part then an image_url part (M242).
func TestEncodeRequest_ImageContentParts(t *testing.T) {
	du := "data:image/png;base64,QUJD"
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "describe", Images: []string{du}}}
	body, err := encodeRequest("gpt-x", "", msgs, nil, 100, false, agent.Params{}, nil)
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	ms := decodeMessages(t, body)
	if len(ms) != 1 {
		t.Fatalf("want 1 message, got %d", len(ms))
	}
	var parts []oaContentPart
	if err := json.Unmarshal(ms[0].Content, &parts); err != nil {
		t.Fatalf("content is not a parts array: %v (%s)", err, ms[0].Content)
	}
	if len(parts) != 2 {
		t.Fatalf("want 2 parts (text,image_url), got %d: %+v", len(parts), parts)
	}
	if parts[0].Type != "text" || parts[0].Text != "describe" {
		t.Errorf("first part not the text: %+v", parts[0])
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL == nil || parts[1].ImageURL.URL != du {
		t.Errorf("second part not the image_url: %+v", parts[1])
	}
}

// A text-only user message keeps the plain-string content form (no drift).
func TestEncodeRequest_TextOnlyStaysString(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hello"}}
	body, err := encodeRequest("gpt-x", "", msgs, nil, 100, false, agent.Params{}, nil)
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	ms := decodeMessages(t, body)
	var s string
	if err := json.Unmarshal(ms[0].Content, &s); err != nil {
		t.Fatalf("content is not a string: %v (%s)", err, ms[0].Content)
	}
	if s != "hello" {
		t.Errorf("content = %q, want hello", s)
	}
}

// A non-URL attachment (legacy bare filename) is skipped, so the message stays
// a plain-string content rather than carrying an invalid image_url.
func TestEncodeRequest_SkipsNonURLImage(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi", Images: []string{"photo.png"}}}
	body, err := encodeRequest("gpt-x", "", msgs, nil, 100, false, agent.Params{}, nil)
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	ms := decodeMessages(t, body)
	var s string
	if err := json.Unmarshal(ms[0].Content, &s); err != nil {
		t.Errorf("want string content for a skipped non-URL image, got %s", ms[0].Content)
	}
	if s != "hi" {
		t.Errorf("content = %q, want hi", s)
	}
}

// A tool-call-only assistant message still omits "content" entirely — the any
// retype must not regress the omitempty wire shape OpenAI expects.
func TestEncodeRequest_ToolCallAssistantOmitsContent(t *testing.T) {
	msgs := []agent.Message{{
		Role:      agent.RoleAssistant,
		ToolCalls: []agent.ToolCall{{ID: "c1", Name: "ls", Input: json.RawMessage(`{}`)}},
	}}
	body, err := encodeRequest("gpt-x", "", msgs, nil, 100, false, agent.Params{}, nil)
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	ms := decodeMessages(t, body)
	if len(ms[0].Content) != 0 {
		t.Errorf("content should be omitted for a tool-call-only assistant, got %s", ms[0].Content)
	}
}

func TestIsImageURL(t *testing.T) {
	for _, ok := range []string{"data:image/png;base64,QUJD", "https://x/y.png", "http://x/y.png"} {
		if !isImageURL(ok) {
			t.Errorf("isImageURL(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"photo.png", "ftp://x/y.png", "", "/abs/path.png"} {
		if isImageURL(bad) {
			t.Errorf("isImageURL(%q) = true, want false", bad)
		}
	}
}
