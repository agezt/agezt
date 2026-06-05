// SPDX-License-Identifier: MIT

package openaiapi

import (
	"encoding/json"
	"testing"
)

// FuzzChatMessageContent hardens the OpenAI-compat message-content parser against
// arbitrary client input. A chat message's `content` is either a plain string or
// an array of typed parts (text / image_url), and the daemon tries several
// json.Unmarshal shapes to flatten it — custom parsing of fully untrusted
// network input. Invariant: text(), images(), and inputImages() never panic on
// any content bytes (they may return empty, but must not crash the API handler).
func FuzzChatMessageContent(f *testing.F) {
	f.Add([]byte(`"hello world"`))
	f.Add([]byte(`[{"type":"text","text":"hi"}]`))
	f.Add([]byte(`[{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]`))
	f.Add([]byte(`[{"type":"image_url","image_url":"http://x/y.png"}]`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte(`[{"type":"text"}]`))

	f.Fuzz(func(t *testing.T, content []byte) {
		m := chatMessage{Role: "user", Content: json.RawMessage(content)}
		_ = m.text()
		_ = m.images()
		_ = m.inputImages()
	})
}
