// SPDX-License-Identifier: MIT

package anthropic

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// A user message carrying an image data: URL is encoded as an Anthropic
// type=image content block (base64 source) placed before the text block (M241).
func TestEncodeRequest_ImageBlock(t *testing.T) {
	raw := []byte{0x89, 0x50, 0x4e, 0x47, 1, 2, 3, 4}
	b64 := base64.StdEncoding.EncodeToString(raw)
	msgs := []agent.Message{{
		Role:    agent.RoleUser,
		Content: "describe this",
		Images:  []string{"data:image/png;base64," + b64},
	}}

	body, err := encodeRequest("claude-x", "", msgs, nil, 100, 0, agent.Params{}, nil)
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	var got anthRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(got.Messages))
	}
	blocks := got.Messages[0].Content
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks (image,text), got %d: %+v", len(blocks), blocks)
	}
	if blocks[0].Type != "image" || blocks[0].Source == nil {
		t.Fatalf("first block is not an image: %+v", blocks[0])
	}
	if blocks[0].Source.Type != "base64" {
		t.Errorf("source type = %q, want base64", blocks[0].Source.Type)
	}
	if blocks[0].Source.MediaType != "image/png" {
		t.Errorf("media type = %q, want image/png", blocks[0].Source.MediaType)
	}
	if blocks[0].Source.Data != b64 {
		t.Errorf("source data = %q, want %q", blocks[0].Source.Data, b64)
	}
	if blocks[1].Type != "text" || blocks[1].Text != "describe this" {
		t.Errorf("second block is not the text: %+v", blocks[1])
	}
}

// The streaming path (the agent loop's default) shares canonicalToAnth, so an
// image data: URL reaches the model there too (M241).
func TestEncodeStreamRequest_ImageBlock(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff})
	msgs := []agent.Message{{
		Role:    agent.RoleUser,
		Content: "what is this",
		Images:  []string{"data:image/jpeg;base64," + b64},
	}}
	body, err := encodeStreamRequest("claude-x", "", msgs, nil, 100, 0, agent.Params{}, nil)
	if err != nil {
		t.Fatalf("encodeStreamRequest: %v", err)
	}
	var got struct {
		Stream   bool          `json:"stream"`
		Messages []anthMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Stream {
		t.Error("stream flag not set on streaming request")
	}
	blocks := got.Messages[0].Content
	if len(blocks) != 2 || blocks[0].Type != "image" || blocks[0].Source == nil {
		t.Fatalf("want image+text blocks, got %+v", blocks)
	}
	if blocks[0].Source.MediaType != "image/jpeg" || blocks[0].Source.Data != b64 {
		t.Errorf("bad image source: %+v", blocks[0].Source)
	}
}

// A legacy bare filename (not a data URL) has no deliverable payload, so it is
// skipped — the message still encodes with its text block, no image block.
func TestEncodeRequest_SkipsNonDataURLImage(t *testing.T) {
	msgs := []agent.Message{{
		Role:    agent.RoleUser,
		Content: "hi",
		Images:  []string{"photo.png"},
	}}
	body, err := encodeRequest("claude-x", "", msgs, nil, 100, 0, agent.Params{}, nil)
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	var got anthRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	blocks := got.Messages[0].Content
	if len(blocks) != 1 || blocks[0].Type != "text" {
		t.Fatalf("want a single text block, got %+v", blocks)
	}
}

// parseImageDataURL splits a data: URL and rejects malformed input.
func TestParseImageDataURL(t *testing.T) {
	mt, data, ok := parseImageDataURL("data:image/webp;base64,QUJD")
	if !ok || mt != "image/webp" || data != "QUJD" {
		t.Errorf("parse = (%q,%q,%v), want (image/webp,QUJD,true)", mt, data, ok)
	}
	for _, bad := range []string{
		"photo.png",                 // bare filename
		"data:image/png,QUJD",       // no ;base64
		"data:;base64,QUJD",         // empty media type
		"data:image/png;base64,",    // empty payload
		"http://x/y.png",            // not a data URL
		"data:image/png;base64QUJD", // no comma
	} {
		if _, _, ok := parseImageDataURL(bad); ok {
			t.Errorf("parseImageDataURL(%q) ok=true, want false", bad)
		}
	}
}
