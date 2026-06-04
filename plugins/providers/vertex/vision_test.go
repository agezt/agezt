// SPDX-License-Identifier: MIT

package vertex

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// Anthropic-on-Vertex: a user message's image data: URL becomes a type=image
// block before the text block (M245).
func TestEncodeAnthropicOnVertex_ImageBlock(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47})
	msgs := []agent.Message{{
		Role:    agent.RoleUser,
		Content: "describe",
		Images:  []string{"data:image/png;base64," + b64},
	}}
	body, err := encodeAnthropicOnVertexRequest("", msgs, nil, 100, false)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got struct {
		Messages []anthVxMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	blocks := got.Messages[0].Content
	if len(blocks) != 2 || blocks[0].Type != "image" || blocks[0].Source == nil {
		t.Fatalf("want image+text blocks, got %+v", blocks)
	}
	if blocks[0].Source.MediaType != "image/png" || blocks[0].Source.Data != b64 {
		t.Errorf("bad image source: %+v", blocks[0].Source)
	}
	if blocks[1].Type != "text" || blocks[1].Text != "describe" {
		t.Errorf("second block not text: %+v", blocks[1])
	}
}

// Gemini-on-Vertex: a user message's image data: URL becomes an inlineData part
// before the text part (M245).
func TestEncodeGeminiOnVertex_InlineImageData(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff})
	msgs := []agent.Message{{
		Role:    agent.RoleUser,
		Content: "what is this",
		Images:  []string{"data:image/jpeg;base64," + b64},
	}}
	body, err := encodeRequest("", msgs, nil, 100, false)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got vxRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	parts := got.Contents[0].Parts
	if len(parts) != 2 || parts[0].InlineData == nil {
		t.Fatalf("want inlineData+text parts, got %+v", parts)
	}
	if parts[0].InlineData.MimeType != "image/jpeg" || parts[0].InlineData.Data != b64 {
		t.Errorf("bad inlineData: %+v", parts[0].InlineData)
	}
	if parts[1].Text != "what is this" {
		t.Errorf("second part not text: %+v", parts[1])
	}
}

// Non-data-URL attachments are skipped on both Vertex encoders.
func TestEncodeOnVertex_SkipsNonDataURLImage(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi", Images: []string{"photo.png"}}}

	ab, err := encodeAnthropicOnVertexRequest("", msgs, nil, 100, false)
	if err != nil {
		t.Fatalf("anthropic encode: %v", err)
	}
	var ag struct {
		Messages []anthVxMessage `json:"messages"`
	}
	_ = json.Unmarshal(ab, &ag)
	if b := ag.Messages[0].Content; len(b) != 1 || b[0].Type != "text" {
		t.Errorf("anthropic: want single text block, got %+v", b)
	}

	gb, err := encodeRequest("", msgs, nil, 100, false)
	if err != nil {
		t.Fatalf("gemini encode: %v", err)
	}
	var gg vxRequest
	_ = json.Unmarshal(gb, &gg)
	if p := gg.Contents[0].Parts; len(p) != 1 || p[0].InlineData != nil || p[0].Text != "hi" {
		t.Errorf("gemini: want single text part, got %+v", p)
	}
}

func TestParseImageDataURL(t *testing.T) {
	mt, data, ok := parseImageDataURL("data:image/webp;base64,QUJD")
	if !ok || mt != "image/webp" || data != "QUJD" {
		t.Errorf("parse = (%q,%q,%v), want (image/webp,QUJD,true)", mt, data, ok)
	}
	for _, bad := range []string{"photo.png", "data:image/png,QUJD", "data:;base64,QUJD", "data:image/png;base64,", "x"} {
		if _, _, ok := parseImageDataURL(bad); ok {
			t.Errorf("parseImageDataURL(%q) ok=true, want false", bad)
		}
	}
}
