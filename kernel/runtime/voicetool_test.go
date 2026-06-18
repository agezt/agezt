// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestDecodeAudio(t *testing.T) {
	want := []byte("OGGDATA")
	b64 := base64.StdEncoding.EncodeToString(want)
	cases := []string{
		b64,
		"data:audio/ogg;base64," + b64,
		b64[:4] + "\n" + b64[4:], // whitespace tolerated
	}
	for _, in := range cases {
		got, err := decodeAudio(in)
		if err != nil {
			t.Fatalf("decodeAudio(%q): %v", in, err)
		}
		if string(got) != string(want) {
			t.Fatalf("decodeAudio(%q) = %q", in, got)
		}
	}
	if _, err := decodeAudio(""); err == nil {
		t.Fatal("empty must error")
	}
	if _, err := decodeAudio("data:audio/ogg,nope"); err == nil {
		t.Fatal("non-base64 data URL must error")
	}
}

// fakeVoice is a Voice with toggleable halves.
type fakeVoice struct{ stt, tts bool }

func (f fakeVoice) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	return "transcript: " + string(audio), nil
}
func (f fakeVoice) Speak(ctx context.Context, text string) ([]byte, string, error) {
	return []byte("AUDIO:" + text), "audio/mpeg", nil
}
func (f fakeVoice) HasSTT() bool { return f.stt }
func (f fakeVoice) HasTTS() bool { return f.tts }

func TestVoiceToolTranscribe(t *testing.T) {
	vt := newVoiceTool(fakeVoice{stt: true})
	in, _ := json.Marshal(voiceToolInput{Op: "transcribe", Audio: base64.StdEncoding.EncodeToString([]byte("hi"))})
	res, err := vt.Invoke(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || res.Output != "transcript: hi" {
		t.Fatalf("res = %+v", res)
	}
	if res.ObservationTrust != agent.ObservationUntrusted {
		t.Fatalf("transcript should be untrusted, got %q", res.ObservationTrust)
	}
}

func TestVoiceToolSpeakNeedsSaver(t *testing.T) {
	// TTS configured but no artifact saver bound → graceful error, not a panic.
	vt := newVoiceTool(fakeVoice{tts: true})
	in, _ := json.Marshal(voiceToolInput{Op: "speak", Text: "hello"})
	res, err := vt.Invoke(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected error without a saver, got %+v", res)
	}

	// With a saver, it persists and reports the ref.
	vt.saveArtifact = func(data []byte) (string, error) { return "art-123", nil }
	res, _ = vt.Invoke(context.Background(), in)
	if res.IsError || res.Output == "" {
		t.Fatalf("speak result = %+v", res)
	}
}

func TestVoiceToolUnconfiguredHalves(t *testing.T) {
	vt := newVoiceTool(fakeVoice{}) // neither half
	in, _ := json.Marshal(voiceToolInput{Op: "transcribe", Audio: "aGk="})
	res, _ := vt.Invoke(context.Background(), in)
	if !res.IsError {
		t.Fatal("transcribe without STT should error")
	}
}
