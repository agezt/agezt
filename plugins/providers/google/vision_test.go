// SPDX-License-Identifier: MIT

package google

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// A user message carrying an image data: URL is encoded as a Gemini inlineData
// part before the text part (M243).
func TestEncodeRequest_InlineImageData(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47, 1, 2})
	msgs := []agent.Message{{
		Role:    agent.RoleUser,
		Content: "describe this",
		Images:  []string{"data:image/png;base64," + b64},
	}}
	body, err := encodeRequest("", msgs, nil, 100, false)
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	var got geminiRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Contents) != 1 {
		t.Fatalf("want 1 content, got %d", len(got.Contents))
	}
	parts := got.Contents[0].Parts
	if len(parts) != 2 {
		t.Fatalf("want 2 parts (image,text), got %d: %+v", len(parts), parts)
	}
	if parts[0].InlineData == nil {
		t.Fatalf("first part is not inlineData: %+v", parts[0])
	}
	if parts[0].InlineData.MimeType != "image/png" || parts[0].InlineData.Data != b64 {
		t.Errorf("bad inlineData: %+v", parts[0].InlineData)
	}
	if parts[1].Text != "describe this" {
		t.Errorf("second part is not the text: %+v", parts[1])
	}
}

// A non-data-URL attachment is skipped, leaving a text-only user content.
func TestEncodeRequest_SkipsNonDataURLImage(t *testing.T) {
	msgs := []agent.Message{{Role: agent.RoleUser, Content: "hi", Images: []string{"photo.png"}}}
	body, err := encodeRequest("", msgs, nil, 100, false)
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	var got geminiRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	parts := got.Contents[0].Parts
	if len(parts) != 1 || parts[0].InlineData != nil || parts[0].Text != "hi" {
		t.Fatalf("want a single text part, got %+v", parts)
	}
}

func TestParseImageDataURL(t *testing.T) {
	mt, data, ok := parseImageDataURL("data:image/webp;base64,QUJD")
	if !ok || mt != "image/webp" || data != "QUJD" {
		t.Errorf("parse = (%q,%q,%v), want (image/webp,QUJD,true)", mt, data, ok)
	}
	for _, bad := range []string{
		"photo.png",
		"data:image/png,QUJD",
		"data:;base64,QUJD",
		"data:image/png;base64,",
		"http://x/y.png",
	} {
		if _, _, ok := parseImageDataURL(bad); ok {
			t.Errorf("parseImageDataURL(%q) ok=true, want false", bad)
		}
	}
}
