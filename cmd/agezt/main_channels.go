// SPDX-License-Identifier: MIT

// Channel helpers extracted from main.go during Day 211 god-file refactor (#43).
// Public API unchanged.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/pulse"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

func visionGate(k *kernelruntime.Kernel, model string, images []string) error {
	return gateVisionWith(k.Catalog(), k.Model(), model, images)
}

// gateVisionWith is the pure core of visionGate (catalog + default model
// injected, so it's testable without a live kernel). eff = model, or
// defaultModel when model is empty. Confirmed-or-reject: an unknown or
// known-but-non-vision model is refused when images are present.
func gateVisionWith(cat *catalog.Catalog, defaultModel, model string, images []string) error {
	if len(images) == 0 {
		return nil
	}
	eff := model
	if eff == "" {
		eff = defaultModel
	}
	visionOK := false
	if cat != nil {
		if _, m := cat.FindModel(eff); m != nil {
			visionOK = m.SupportsVision()
		}
	}
	if !visionOK {
		return fmt.Errorf("model %q does not support vision (image input); attach images only to a vision-capable model", eff)
	}
	return nil
}

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

// voiceReplyEnabled reports whether voice-in→voice-out is on (default yes).
func voiceReplyEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "VOICE_REPLY")))
	return v != "off" && v != "0" && v != "false" && v != "no"
}

// audioExt maps a TTS MIME type to a file extension for the outbound clip.
func audioExt(mime string) string {
	switch {
	case strings.Contains(mime, "ogg"), strings.Contains(mime, "opus"):
		return ".ogg"
	case strings.Contains(mime, "mpeg"), strings.Contains(mime, "mp3"):
		return ".mp3"
	case strings.Contains(mime, "wav"):
		return ".wav"
	case strings.Contains(mime, "aac"), strings.Contains(mime, "m4a"), strings.Contains(mime, "mp4"):
		return ".m4a"
	default:
		return ".ogg"
	}
}

// persistInboundAudio saves each inbound channel audio clip (voice note) as a
// browsable artifact entry, keyed to the run correlation. Best-effort: a
// decode/store failure for one clip is skipped, never fatal to the run.
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

// persistInboundImages saves each inbound channel image as a browsable artifact
// entry (M822), keyed to the run correlation, with the vision caption (if the
// sidecar ran) attached. Best-effort: a decode/store failure for one image is
// logged-by-omission, never fatal to the run. Returns the new entry ids.
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

// decodeDataURL parses a data: URL (data:<mime>[;base64],<payload>) into its mime
// and decoded bytes. ok=false for anything that isn't a data URL. Base64 is the
// only encoding channels produce for images; a non-base64 payload is returned raw.
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

// extForMime maps the common image mimes to a file extension for the artifact's
// display name; unknown types get no extension.
func extForMime(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "audio/ogg", "audio/opus":
		return ".ogg"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	default:
		return ""
	}
}

// collectChannels reports the configured messaging channels for `agt status`
// (M141), read-only from the same env the buildX functions consume. A channel is
// listed when its token is set; Inbound reflects whether it can actually receive
// and act on commands (Telegram always can; Slack/Discord need a listen addr plus
// the inbound secret/public key), so a half-configured webhook channel shows up
// as outbound-only rather than silently looking active.
// collectChannels derives `agt status`'s configured-channel list from the
// channel manifest registry (LD-7). It used to be a hand-maintained per-kind
// predicate list that had silently drifted to cover 11 of the 34 registered
// kinds; deriving from Manifest.RequiredEnv/AddrEnv/AllowlistEnv/InboundEnv
// means a newly registered channel is status-visible with no edit here.
func collectChannels() []controlplane.ChannelInfo {
	env := func(name string) string { return strings.TrimSpace(os.Getenv(name)) }
	var out []controlplane.ChannelInfo
	for _, m := range channel.Manifests() {
		configured := len(m.RequiredEnv) > 0
		for _, e := range m.RequiredEnv {
			if env(e) == "" {
				configured = false
				break
			}
		}
		// The generic webhook is usable outbound-only with just an outbound
		// URL — the one kind whose "configured" predicate is an OR the
		// manifest's all-required semantics can't express.
		if !configured && m.Kind == "webhook" && env("AGEZT_WEBHOOK_OUTBOUND_URL") != "" {
			configured = true
		}
		if !configured {
			continue
		}
		inbound := m.Duplex
		for _, e := range m.InboundEnv {
			if env(e) == "" {
				inbound = false
				break
			}
		}
		info := controlplane.ChannelInfo{Kind: m.Kind, Inbound: inbound}
		if m.AddrEnv != "" {
			info.Addr = env(m.AddrEnv)
		}
		if m.AllowlistEnv != "" {
			info.Allowlist = len(splitNonEmpty(env(m.AllowlistEnv)))
		}
		out = append(out, info)
	}
	return out
}

// combineSinks tees the configured channel brief sinks (Telegram, Slack, Discord)
// into one Pulse sink. Nil entries are dropped; returns nil when none are configured.
func combineSinks(sinks ...pulse.BriefSink) pulse.BriefSink {
	var live pulse.MultiSink
	for _, s := range sinks {
		if s != nil {
			live = append(live, s)
		}
	}
	switch len(live) {
	case 0:
		return nil
	case 1:
		return live[0]
	default:
		return live
	}
}

// chanInstance is one configured account-instance of a channel kind (multi-account).
type chanInstance struct {
	key  string // channel.InstanceKey(kind, label): bare kind for the default, "kind#label" otherwise
	desc string
	ch   channel.Channel
	sink pulse.BriefSink
}

// wireInstances converts channelwire's built instances (the factory-migrated
// channels, Phase 2.1) into the daemon's chanInstance shape so the existing
// startInstances / allInsts / registerInstances wiring stays untouched.
func wireInstances(insts []channelwire.Instance) []chanInstance {
	var out []chanInstance
	for _, in := range insts {
		out = append(out, chanInstance{key: in.Key, desc: in.Desc, ch: in.Channel, sink: in.Sink})
	}
	return out
}

// startInstances starts each instance's read loop and logs it; logs a single
// "disabled" line for the kind when none are configured.
func startInstances(ctx context.Context, stdout io.Writer, kind, label, disabledHint string, insts []chanInstance) {
	if len(insts) == 0 {
		if disabledHint != "" {
			fmt.Fprintf(stdout, "  %-16s : %s\n", label, disabledHint)
		}
		return
	}
	for _, in := range insts {
		go in.ch.Start(ctx)
		who := in.key
		if who == kind {
			who = "default"
		}
		fmt.Fprintf(stdout, "  %-16s : %s [%s]\n", label, in.desc, who)
	}
}

// instanceSinks collects the non-nil brief sinks across instance groups.
func instanceSinks(groups ...[]chanInstance) []pulse.BriefSink {
	var out []pulse.BriefSink
	for _, g := range groups {
		for _, in := range g {
			if in.sink != nil {
				out = append(out, in.sink)
			}
		}
	}
	return out
}

// registerInstances maps every instance into liveChannels by its instance key.
func registerInstances(live map[string]channel.Channel, groups ...[]chanInstance) {
	for _, g := range groups {
		for _, in := range g {
			live[in.key] = in.ch
		}
	}
}

// instanceMatch returns the instance keys a send target addresses: an exact
// "kind#label" key, or every "kind"/"kind#*" key when target is a bare kind
// (fan-out across all accounts of that kind). For a single-account kind this is
// exactly one key — identical to the pre-multi-account behavior.
func instanceMatch(keys []string, target string) []string {
	if strings.Contains(target, "#") {
		for _, k := range keys {
			if k == target {
				return []string{target}
			}
		}
		return nil
	}
	var out []string
	for _, k := range keys {
		if base, _, _ := strings.Cut(k, "#"); base == target {
			out = append(out, k)
		}
	}
	return out
}

// liveChannelKeys returns the instance keys of the live channel map (used to
// record per-account live state for the Channels UI).
func liveChannelKeys(live map[string]channel.Channel) []string {
	out := make([]string, 0, len(live))
	for k := range live {
		out = append(out, k)
	}
	return out
}

// briefSink returns the Pulse sink: the log sink alone, or teed with extra
// (Telegram) when configured.
func briefSink(stdout io.Writer, extra pulse.BriefSink) pulse.BriefSink {
	log := pulse.LogSink{W: stdout}
	if extra == nil {
		return log
	}
	return pulse.MultiSink{log, extra}
}

// splitNonEmpty splits a comma list, trimming and dropping blanks.
func splitNonEmpty(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// startReflectTicker starts a periodic reflection pass when AGEZT_REFLECT_EVERY
// is a valid positive duration, on the daemon ctx (so halt/shutdown stop it).
// Returns a banner description, or "" when no timer is configured. Mirrors the
// Pulse ticker lifecycle.
