// SPDX-License-Identifier: MIT
//
// /v1/chat/completions request types: chatRequest + chatRespFormat +
// wantsJSON + streamOptions + chatMessage + text/images/inputImages methods
// + imagesFromMessages extractor. Split from openaiapi_server.go during
// Day 211 god-file refactor (#35). Public API unchanged.
package openaiapi

import (
	"encoding/json"
	"strings"
)


type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Stream         bool            `json:"stream"`
	StreamOptions  *streamOptions  `json:"stream_options,omitempty"`
	ResponseFormat *chatRespFormat `json:"response_format,omitempty"`
}

// chatRespFormat is OpenAI's response_format request object. We honour
// json_object and json_schema (both mean "structured JSON") by switching the
// run to JSON mode (M314); "text" (or absent) is the default free-form output.
type chatRespFormat struct {
	Type string `json:"type"` // "text" | "json_object" | "json_schema"
}

// wantsJSON reports whether a response_format asks for structured JSON output.
func (f *chatRespFormat) wantsJSON() bool {
	return f != nil && (f.Type == "json_object" || f.Type == "json_schema")
}

// streamOptions mirrors OpenAI's stream_options. IncludeUsage requests a final
// usage-only chunk at the end of a stream (M237) — cost-tracking clients and the
// OpenAI SDK rely on it when set.
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// text flattens OpenAI message content, which is either a plain string or an
// array of typed parts ([{type:"text", text:"..."}]). Non-text parts (images)
// are ignored — Agezt's intent is text.
func (m chatMessage) text() string {
	if len(m.Content) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return strings.TrimSpace(s)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(m.Content, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Text != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(p.Text)
			}
		}
		return strings.TrimSpace(b.String())
	}
	return ""
}

// images extracts image attachment URLs from a message's content parts. OpenAI
// Chat Completions carries an image as {type:"image_url", image_url:{url:...}};
// the url is a data: URL or an http(s) URL. Returns nil for string content or
// a part list with no images.
func (m chatMessage) images() []string {
	if len(m.Content) == 0 {
		return nil
	}
	var parts []struct {
		Type     string `json:"type"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if json.Unmarshal(m.Content, &parts) != nil {
		return nil
	}
	var urls []string
	for _, p := range parts {
		if p.Type == "image_url" && p.ImageURL.URL != "" {
			urls = append(urls, p.ImageURL.URL)
		}
	}
	return urls
}

// inputImages extracts Responses-API `input_image` part URLs. There the
// image_url field is a bare string (the documented Responses shape); some SDKs
// send the Chat-style {url} object, so both are tolerated (M250). This is
// distinct from images(), which reads Chat Completions' `image_url` parts.
func (m chatMessage) inputImages() []string {
	if len(m.Content) == 0 {
		return nil
	}
	var parts []struct {
		Type     string          `json:"type"`
		ImageURL json.RawMessage `json:"image_url"`
	}
	if json.Unmarshal(m.Content, &parts) != nil {
		return nil
	}
	var urls []string
	for _, p := range parts {
		if p.Type != "input_image" || len(p.ImageURL) == 0 {
			continue
		}
		var s string
		if json.Unmarshal(p.ImageURL, &s) == nil && s != "" {
			urls = append(urls, s)
			continue
		}
		var o struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(p.ImageURL, &o) == nil && o.URL != "" {
			urls = append(urls, o.URL)
		}
	}
	return urls
}

// imagesFromMessages collects image attachment URLs from the user messages so a
// multimodal chat completion forwards its images to the run; Agezt's providers
// turn each into the model's native image input (M246). The kernel still gates
// the model's vision capability at the provider call.
func imagesFromMessages(msgs []chatMessage) []string {
	var urls []string
	for _, m := range msgs {
		if strings.EqualFold(m.Role, "user") {
			urls = append(urls, m.images()...)
		}
	}
	return urls
}
