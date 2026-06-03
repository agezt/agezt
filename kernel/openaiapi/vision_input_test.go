// SPDX-License-Identifier: MIT

package openaiapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A multimodal chat completion (text + image_url parts) forwards the image URL
// to the run, so a vision model receives it (M246).
func TestChat_ForwardsImageURL(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "a cat"}
	s := newAPIServer(t, eng, "secret")
	du := "data:image/png;base64,QUJD"
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":[` +
		`{"type":"text","text":"what is this?"},` +
		`{"type":"image_url","image_url":{"url":"` + du + `"}}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(eng.ranImages) != 1 || eng.ranImages[0] != du {
		t.Errorf("ranImages=%v, want [%s]", eng.ranImages, du)
	}
	if eng.ranIntent != "what is this?" {
		t.Errorf("ranIntent=%q, want %q", eng.ranIntent, "what is this?")
	}
}

// An image-only message (no text part) is accepted and runs with a default
// instruction rather than being rejected as empty.
func TestChat_ImageOnlyMessage(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "ok"}
	s := newAPIServer(t, eng, "secret")
	du := "data:image/jpeg;base64,QQ=="
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":[` +
		`{"type":"image_url","image_url":{"url":"` + du + `"}}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(eng.ranImages) != 1 || eng.ranImages[0] != du {
		t.Errorf("ranImages=%v, want [%s]", eng.ranImages, du)
	}
	if strings.TrimSpace(eng.ranIntent) == "" {
		t.Error("image-only request ran with an empty intent")
	}
}

// A request with neither text nor images is still rejected.
func TestChat_EmptyContentRejected(t *testing.T) {
	eng := &fakeEngine{model: "m", answer: "x"}
	s := newAPIServer(t, eng, "secret")
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":""}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

// imagesFromMessages pulls image_url parts from user messages only.
func TestImagesFromMessages(t *testing.T) {
	msgs := []chatMessage{
		{Role: "system", Content: json.RawMessage(`"be brief"`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA"}}]`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"image_url","image_url":{"url":"data:image/png;base64,ZZ"}}]`)},
	}
	got := imagesFromMessages(msgs)
	if len(got) != 1 || got[0] != "data:image/png;base64,AA" {
		t.Errorf("images=%v, want [data:image/png;base64,AA] (user only)", got)
	}
}
