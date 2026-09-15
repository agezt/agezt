// SPDX-License-Identifier: MIT
//
// cmd/agezt channel handler + inbound-media persistence
// (makeChannelHandler, persistInboundAudio, persistInboundImages, decodeDataURL).
// Extracted from main_channels.go during Day 211 god-file refactor (#66).
// Public API unchanged.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/channel"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

func makeChannelHandler(k *kernelruntime.Kernel) channel.InboundHandler {
	limit := channelHistoryLimit()
	return func(hctx context.Context, msg channel.UnifiedMessage, corr string) (channel.Reply, error) {
		intent := msg.Text
		if h := channel.ConversationHistory(k.Journal(), msg.ChannelKind, msg.ChannelID, msg.ThreadID, msg.Sender, limit); h != "" {
			intent = h
		}
		// Inbound image attachments (M247): forward them to the run the same way
		// the control plane and OpenAI API do, so a photo sent to the bot reaches
		// a vision model. An image with no caption gets a default instruction.
		if len(msg.Images) > 0 {
			var caption string
			if err := visionGate(k, "", msg.Images); err != nil {
				// The active model can't see images. Vision SIDECAR (M821): a keyed
				// vision model describes the image and we inject that text into the
				// run, so a non-vision primary still "reads" the photo instead of
				// failing. If NO vision model is keyed, persist the image anyway
				// (so it's not lost) and surface the clear gate error.
				c, derr := k.DescribeImages(hctx, corr, msg.Images, "")
				if derr != nil {
					if errors.Is(derr, kernelruntime.ErrNoVisionModel) {
						persistInboundImages(k, msg, corr, "")
						return channel.Reply{}, err
					}
					return channel.Reply{}, derr
				}
				caption = c
				if strings.TrimSpace(intent) == "" {
					intent = "Describe the attached image(s)."
				}
				intent += "\n\n[Image description (analyzed by a vision model):\n" + caption + "\n]"
			} else {
				hctx = kernelruntime.WithImages(hctx, msg.Images)
				if strings.TrimSpace(intent) == "" {
					intent = "Describe the attached image(s)."
				}
			}
			// Persist the inbound image(s) as browsable artifacts (M822) — keyed to
			// this run's correlation, with the vision caption (if any) attached.
			persistInboundImages(k, msg, corr, caption)
		}
		// Inbound voice notes: transcribe them so a voice message "just works" —
		// the agent reads the transcript like any text. Best-effort: if no STT is
		// configured, or transcription fails, the audio is still persisted as an
		// artifact (below) and the run proceeds on whatever text there was.
		if len(msg.Audio) > 0 {
			if v := k.Voice(); v != nil && v.HasSTT() {
				var transcripts []string
				for _, du := range msg.Audio {
					_, data, ok := decodeDataURL(du)
					if !ok || len(data) == 0 {
						continue
					}
					if txt, terr := v.Transcribe(hctx, data, "voice.ogg"); terr == nil && strings.TrimSpace(txt) != "" {
						transcripts = append(transcripts, strings.TrimSpace(txt))
					}
				}
				if len(transcripts) > 0 {
					joined := strings.Join(transcripts, "\n")
					if strings.TrimSpace(intent) == "" {
						intent = joined
					} else {
						intent += "\n\n[Voice message transcript:\n" + joined + "\n]"
					}
				}
			}
			persistInboundAudio(k, msg, corr)
		}
		text, rerr := k.RunWith(hctx, corr, intent)
		reply := channel.Reply{Text: text}
		// Voice-in → voice-out: if the user sent a voice message and TTS is
		// configured, speak the answer back as an audio attachment so the
		// conversation stays in voice (opt out with AGEZT_VOICE_REPLY=off).
		if rerr == nil && len(msg.Audio) > 0 && strings.TrimSpace(text) != "" && voiceReplyEnabled() {
			if v := k.Voice(); v != nil && v.HasTTS() {
				if audio, mime, serr := v.Speak(hctx, text); serr == nil && len(audio) > 0 {
					reply.Attachments = append(reply.Attachments, channel.Attachment{
						Kind: "audio", Data: audio, MIME: mime, Filename: "reply" + audioExt(mime),
					})
				}
			}
		}
		return reply, rerr
	}
}
func persistInboundAudio(k *kernelruntime.Kernel, msg channel.UnifiedMessage, corr string) {
	idx := k.ArtifactIndex()
	if idx == nil || len(msg.Audio) == 0 {
		return
	}
	now := time.Now().UnixMilli()
	for n, du := range msg.Audio {
		mime, data, ok := decodeDataURL(du)
		if !ok || len(data) == 0 {
			continue
		}
		_, _ = idx.PutEntry(artifact.Entry{
			Kind:   "audio",
			Source: msg.ChannelKind,
			Sender: msg.Sender,
			Corr:   corr,
			Mime:   mime,
			Name:   fmt.Sprintf("%s-audio-%d%s", msg.ChannelKind, n+1, extForMime(mime)),
		}, data, now)
	}
}
func persistInboundImages(k *kernelruntime.Kernel, msg channel.UnifiedMessage, corr, caption string) []string {
	idx := k.ArtifactIndex()
	if idx == nil || len(msg.Images) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	var ids []string
	for n, du := range msg.Images {
		mime, data, ok := decodeDataURL(du)
		if !ok || len(data) == 0 {
			continue
		}
		e, err := idx.PutEntry(artifact.Entry{
			Kind:    "image",
			Source:  msg.ChannelKind,
			Sender:  msg.Sender,
			Corr:    corr,
			Mime:    mime,
			Name:    fmt.Sprintf("%s-image-%d%s", msg.ChannelKind, n+1, extForMime(mime)),
			Caption: caption,
		}, data, now)
		if err == nil {
			ids = append(ids, e.ID)
		}
	}
	return ids
}
func decodeDataURL(s string) (mime string, data []byte, ok bool) {
	if !strings.HasPrefix(s, "data:") {
		return "", nil, false
	}
	rest := s[len("data:"):]
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return "", nil, false
	}
	meta, payload := rest[:comma], rest[comma+1:]
	mime = meta
	base64Encoded := false
	if i := strings.IndexByte(meta, ';'); i >= 0 {
		mime = meta[:i]
		base64Encoded = strings.Contains(meta[i:], "base64")
	}
	if base64Encoded {
		b, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", nil, false
		}
		return mime, b, true
	}
	return mime, []byte(payload), true
}
