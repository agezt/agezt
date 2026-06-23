// SPDX-License-Identifier: MIT

package ollama

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// TestEncodeRequest_VisionImagesAsRawBase64 (M309): a user message's image
// attachments (RFC 2397 data: URLs, as the CLI sends) are forwarded to Ollama as
// RAW base64 in the `images` array — the data: prefix and media type stripped,
// since Ollama sniffs the format itself. Entries Ollama can't use (an http URL it
// won't fetch, a bare filename) are skipped. This is what lets local vision
// models — llava, llama3.2-vision, moondream — actually see the image.
func TestEncodeRequest_VisionImagesAsRawBase64(t *testing.T) {
	// A 1x1 PNG as a data: URL.
	const b64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	dataURL := "data:image/png;base64," + b64

	body, err := encodeRequest("llava", "",
		[]agent.Message{{
			Role:    agent.RoleUser,
			Content: "what is in this image?",
			Images:  []string{dataURL, "https://example.com/cant-fetch.png", "legacy-bare.png"},
		}}, nil, 0, false, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var req struct {
		Messages []struct {
			Role    string   `json:"role"`
			Content string   `json:"content"`
			Images  []string `json:"images"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("messages=%d want 1", len(req.Messages))
	}
	u := req.Messages[0]
	if u.Role != "user" || u.Content != "what is in this image?" {
		t.Errorf("user message = %+v", u)
	}
	// Only the data: URL has a deliverable payload; the http URL and bare
	// filename are skipped.
	if len(u.Images) != 1 {
		t.Fatalf("images=%d want 1 (only the data URL survives); got %v", len(u.Images), u.Images)
	}
	if u.Images[0] != b64 {
		t.Errorf("image[0] should be the raw base64 (data: prefix stripped); got %q", u.Images[0])
	}
}

// TestEncodeRequest_NoImagesOmitsField: a text-only message must not emit an
// empty `images` array (omitempty), so non-vision runs are byte-for-byte
// unchanged.
func TestEncodeRequest_NoImagesOmitsField(t *testing.T) {
	body, err := encodeRequest("llama3", "",
		[]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil, 0, false, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"images"`) {
		t.Errorf("text-only message must omit the images field: %s", body)
	}
}
