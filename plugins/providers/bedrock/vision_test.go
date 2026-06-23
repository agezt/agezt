// SPDX-License-Identifier: MIT

package bedrock

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// A user message carrying an image data: URL is encoded as an Anthropic-on-
// Bedrock type=image content block before the text block (M244).
func TestEncodeAnthropicOnBedrock_ImageBlock(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47})
	msgs := []agent.Message{{
		Role:    agent.RoleUser,
		Content: "describe this",
		Images:  []string{"data:image/png;base64," + b64},
	}}
	body, err := encodeAnthropicOnBedrockRequest("", msgs, nil, 100, agent.Params{}, nil)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got struct {
		Messages []anthMessage `json:"messages"`
	}
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
	if blocks[0].Source.Type != "base64" || blocks[0].Source.MediaType != "image/png" || blocks[0].Source.Data != b64 {
		t.Errorf("bad image source: %+v", blocks[0].Source)
	}
	if blocks[1].Type != "text" || blocks[1].Text != "describe this" {
		t.Errorf("second block is not the text: %+v", blocks[1])
	}
}

// A non-data-URL attachment is skipped, leaving a text-only user block.
func TestEncodeAnthropicOnBedrock_SkipsNonDataURLImage(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi", Images: []string{"photo.png"}}}
	body, err := encodeAnthropicOnBedrockRequest("", msgs, nil, 100, agent.Params{}, nil)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got struct {
		Messages []anthMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	blocks := got.Messages[0].Content
	if len(blocks) != 1 || blocks[0].Type != "text" {
		t.Fatalf("want a single text block, got %+v", blocks)
	}
}

func TestParseImageDataURL(t *testing.T) {
	mt, data, ok := parseImageDataURL("data:image/gif;base64,QUJD")
	if !ok || mt != "image/gif" || data != "QUJD" {
		t.Errorf("parse = (%q,%q,%v), want (image/gif,QUJD,true)", mt, data, ok)
	}
	for _, bad := range []string{"photo.png", "data:image/png,QUJD", "data:;base64,QUJD", "data:image/png;base64,", "http://x/y"} {
		if _, _, ok := parseImageDataURL(bad); ok {
			t.Errorf("parseImageDataURL(%q) ok=true, want false", bad)
		}
	}
}
