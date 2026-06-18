// SPDX-License-Identifier: MIT

package runtime

// Agent-facing voice tool: lets a running agent HEAR (transcribe inbound audio
// to text) and SPEAK (synthesize a spoken reply). It drives the daemon-injected
// voice adapter (runtime.Config.Voice — typically the OpenAI-compatible STT/TTS
// plugin). Transcription takes a data: URL (or bare base64) so a channel can
// hand a voice note straight through without a network fetch; synthesis returns
// the audio saved as an artifact, so a channel or the operator can play it back.
// The kernel never imports the plugin — only this interface.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)

// Voice is the seam the `voice` tool drives. Implemented by the daemon's voice
// adapter; either half may be unconfigured (reported via HasSTT/HasTTS).
type Voice interface {
	// Transcribe turns audio bytes into text (speech-to-text).
	Transcribe(ctx context.Context, audio []byte, filename string) (string, error)
	// Speak turns text into audio bytes + their MIME type (text-to-speech).
	Speak(ctx context.Context, text string) (audio []byte, mime string, err error)
	// HasSTT / HasTTS report which halves are configured.
	HasSTT() bool
	HasTTS() bool
}

// voiceTool implements agent.Tool over a Voice adapter. saveArtifact is bound to
// the kernel's artifact store after Open (the audio bytes are persisted there,
// never returned inline to the model).
type voiceTool struct {
	voice        Voice
	saveArtifact func(data []byte) (string, error)
}

func newVoiceTool(v Voice) *voiceTool { return &voiceTool{voice: v} }

func (t *voiceTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "voice",
		Description: "Hear and speak. op=transcribe converts inbound audio to text — pass the audio as a data: URL " +
			"(e.g. a voice note delivered by a channel) or bare base64. op=speak synthesizes a spoken reply from text " +
			"and saves it as an audio artifact you can attach to a message. Use transcribe to understand voice messages; " +
			"use speak only when a spoken reply is actually wanted.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "op": {"type": "string", "enum": ["transcribe", "speak"], "description": "transcribe audio→text, or speak text→audio"},
    "audio": {"type": "string", "description": "op=transcribe: the audio as a data: URL or bare base64"},
    "filename": {"type": "string", "description": "op=transcribe: optional source filename (hints the codec, e.g. note.ogg)"},
    "text": {"type": "string", "description": "op=speak: the text to synthesize"}
  },
  "required": ["op"]
}`),
		Effect: agent.ToolEffect{Class: agent.EffectReadOnly},
	}
}

type voiceToolInput struct {
	Op       string `json:"op"`
	Audio    string `json:"audio"`
	Filename string `json:"filename"`
	Text     string `json:"text"`
}

func (t *voiceTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	if t.voice == nil {
		return agent.Result{Output: "voice is not available on this daemon", IsError: true}, nil
	}
	var in voiceToolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	switch strings.TrimSpace(in.Op) {
	case "transcribe":
		return t.transcribe(ctx, in)
	case "speak":
		return t.speak(ctx, in)
	default:
		return agent.Result{Output: fmt.Sprintf("unknown op %q (use transcribe|speak)", in.Op), IsError: true}, nil
	}
}

func (t *voiceTool) transcribe(ctx context.Context, in voiceToolInput) (agent.Result, error) {
	if !t.voice.HasSTT() {
		return agent.Result{Output: "transcription is not configured (set AGEZT_STT_URL + AGEZT_STT_MODEL)", IsError: true}, nil
	}
	audio, err := decodeAudio(in.Audio)
	if err != nil {
		return agent.Result{Output: "invalid audio: " + err.Error(), IsError: true}, nil
	}
	text, err := t.voice.Transcribe(ctx, audio, in.Filename)
	if err != nil {
		return agent.Result{Output: "transcribe failed: " + err.Error(), IsError: true}, nil
	}
	if text == "" {
		return agent.Result{Output: "(no speech detected)", ObservationTrust: agent.ObservationUntrusted, ObservationSource: "voice:transcription"}, nil
	}
	// The transcript is external content — mark it untrusted so the loop renders
	// it as data, never as instructions.
	return agent.Result{Output: text, ObservationTrust: agent.ObservationUntrusted, ObservationSource: "voice:transcription"}, nil
}

func (t *voiceTool) speak(ctx context.Context, in voiceToolInput) (agent.Result, error) {
	if !t.voice.HasTTS() {
		return agent.Result{Output: "synthesis is not configured (set AGEZT_TTS_URL + AGEZT_TTS_MODEL)", IsError: true}, nil
	}
	if strings.TrimSpace(in.Text) == "" {
		return agent.Result{Output: "op=speak needs text", IsError: true}, nil
	}
	audio, mime, err := t.voice.Speak(ctx, in.Text)
	if err != nil {
		return agent.Result{Output: "speak failed: " + err.Error(), IsError: true}, nil
	}
	if t.saveArtifact == nil {
		return agent.Result{Output: fmt.Sprintf("synthesized %d bytes of %s, but artifact storage is unavailable to persist it", len(audio), mime), IsError: true}, nil
	}
	ref, err := t.saveArtifact(audio)
	if err != nil {
		return agent.Result{Output: "saved synthesis failed: " + err.Error(), IsError: true}, nil
	}
	return agent.Result{Output: fmt.Sprintf("spoke %d byte(s) of %s, saved as artifact %s", len(audio), mime, ref)}, nil
}

// decodeAudio accepts a data: URL ("data:audio/ogg;base64,...."), a bare base64
// string, or base64 with surrounding whitespace, and returns the raw bytes.
func decodeAudio(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty audio")
	}
	if strings.HasPrefix(s, "data:") {
		i := strings.Index(s, ",")
		if i < 0 {
			return nil, fmt.Errorf("malformed data URL")
		}
		meta, payload := s[5:i], s[i+1:]
		if !strings.Contains(meta, "base64") {
			return nil, fmt.Errorf("only base64 data URLs are supported")
		}
		s = payload
	}
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	return base64.StdEncoding.DecodeString(s)
}
