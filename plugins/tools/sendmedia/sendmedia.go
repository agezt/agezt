// SPDX-License-Identifier: MIT

// Package sendmedia is the agent's proactive media-messaging tool — the
// attachment-carrying sibling of `notify`. It lets a running agent push an
// image, a voice clip or a file (an artifact it produced this run — a chart, a
// rendered image, a spoken summary) out a configured chat channel, alongside an
// optional caption.
//
// Security (mirrors notify, SPEC-04 §1.7): outbound from an agent is an
// injection-adjacent surface, so destinations are PINNED to the operator's own
// pre-configured allowlist — the agent supplies only the artifact ref, an
// optional caption and optionally which channel kind, NEVER a recipient id. The
// payload is referenced by content-addressed artifact ref (not raw bytes), so a
// prompt-injected agent can only ever send blobs that already exist in the store
// to the operator's own chats. Gated by the same Edict capability as notify.
//
// Lifecycle: like notify, the tool is registered unbound BEFORE the kernel (and
// its channels) start — so the tool map is never written while the agent loop
// reads it — then Bind wires the media sender, the operator allowlist and the
// artifact resolver once channels exist. Bind/Invoke synchronize on a mutex.
package sendmedia

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/channel"
)

// MediaSender delivers an attachment (plus optional caption) out a named channel
// kind to a specific channel/chat id. The daemon wires it to the live channels'
// Send methods.
type MediaSender func(ctx context.Context, kind, channelID, text string, atts []channel.Attachment) error

// ArtifactResolver resolves a content-addressed artifact ref to its bytes.
type ArtifactResolver func(ref string) ([]byte, error)

// Tool implements agent.Tool. Constructed unbound via New; the daemon calls Bind
// once the live channels exist. Until bound, Invoke returns a clean error.
type Tool struct {
	mu      sync.RWMutex
	send    MediaSender
	resolve ArtifactResolver
	targets map[string][]string // channel kind → operator's allowlisted ids
}

// New returns an unbound sendmedia Tool. Call Bind before runs begin.
func New() *Tool { return &Tool{} }

// Bind wires the tool to the media sender, the operator's per-kind allowlist ids
// and the artifact resolver. Kinds with no ids are dropped. A nil sender,
// resolver or empty targets leaves the tool effectively disabled.
func (t *Tool) Bind(send MediaSender, targets map[string][]string, resolve ArtifactResolver) {
	pruned := map[string][]string{}
	for kind, ids := range targets {
		if len(ids) > 0 {
			pruned[kind] = append([]string(nil), ids...)
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.send = send
	t.resolve = resolve
	t.targets = pruned
}

func (t *Tool) snapshot() (MediaSender, ArtifactResolver, map[string][]string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.send, t.resolve, t.targets
}

func kinds(targets map[string][]string) []string {
	ks := make([]string, 0, len(targets))
	for k := range targets {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func (t *Tool) Definition() agent.ToolDef {
	_, _, targets := t.snapshot()
	avail := strings.Join(kinds(targets), ", ")
	if avail == "" {
		avail = "(none configured yet)"
	}
	return agent.ToolDef{
		Name: "send_media",
		Description: "Send an image, voice clip or file to the operator over a configured chat channel " +
			"(" + avail + "). Reference the content by its artifact ref (e.g. an image you rendered or a " +
			"clip you produced this run) and optionally add a caption. The media goes ONLY to the operator's " +
			"pre-configured chats; you cannot choose arbitrary recipients. Text-only channels receive just the caption.",
		Effect: agent.ToolEffect{
			Class: agent.EffectCompensable,
			PredictedEffects: []string{
				"send an outbound media attachment to the operator's configured channel allowlist",
				"may interrupt or notify the operator outside the current run UI",
			},
			AffectedResources: []string{"operator notification channels: " + avail},
			RollbackNotes:     "Sent media cannot be unsent by the daemon; compensate with a follow-up message if needed.",
			Confidence:        0.8,
		},
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "artifact": {
      "type": "string",
      "description": "Content-addressed artifact ref of the media to send (the bytes must already be in the artifact store)."
    },
    "caption": {
      "type": "string",
      "description": "Optional caption / message text to accompany the media."
    },
    "kind": {
      "type": "string",
      "enum": ["image", "audio", "file"],
      "description": "Optional: how to send the artifact. Omit to auto-detect from the content type."
    },
    "channel": {
      "type": "string",
      "description": "Optional: restrict delivery to one channel kind. Omit to send to all configured channels."
    }
  },
  "required": ["artifact"]
}`),
	}
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	send, resolve, targets := t.snapshot()
	if send == nil || resolve == nil || len(targets) == 0 {
		return agent.Result{Output: "send_media is not configured (no channel with an allowlist)", IsError: true}, nil
	}

	var in struct {
		Artifact string `json:"artifact"`
		Caption  string `json:"caption"`
		Kind     string `json:"kind"`
		Channel  string `json:"channel"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	ref := strings.TrimSpace(in.Artifact)
	if ref == "" {
		return agent.Result{Output: "artifact is required", IsError: true}, nil
	}

	data, err := resolve(ref)
	if err != nil {
		return agent.Result{Output: "could not resolve artifact " + ref + ": " + err.Error(), IsError: true}, nil
	}
	if len(data) == 0 {
		return agent.Result{Output: "artifact " + ref + " is empty", IsError: true}, nil
	}

	att := buildAttachment(data, strings.ToLower(strings.TrimSpace(in.Kind)))
	caption := strings.TrimSpace(in.Caption)

	// Resolve which channel kinds to deliver to.
	var deliver []string
	if k := strings.ToLower(strings.TrimSpace(in.Channel)); k != "" {
		if _, ok := targets[k]; !ok {
			return agent.Result{Output: fmt.Sprintf("channel %q is not configured; available: %s", k, strings.Join(kinds(targets), ", ")), IsError: true}, nil
		}
		deliver = []string{k}
	} else {
		deliver = kinds(targets)
	}

	sent := 0
	var errs []string
	for _, kind := range deliver {
		for _, id := range targets[kind] {
			if err := send(ctx, kind, id, caption, []channel.Attachment{att}); err != nil {
				errs = append(errs, fmt.Sprintf("%s/%s: %v", kind, id, err))
				continue
			}
			sent++
		}
	}

	if sent == 0 {
		return agent.Result{Output: "send_media failed: " + strings.Join(errs, "; "), IsError: true}, nil
	}
	out := fmt.Sprintf("sent %s media to the operator (%d recipient(s) across %s)", att.Kind, sent, strings.Join(deliver, ", "))
	if len(errs) > 0 {
		return agent.Result{Output: out + "; but some deliveries FAILED: " + strings.Join(errs, "; "), IsError: true}, nil
	}
	return agent.Result{Output: out}, nil
}

// buildAttachment classifies the bytes into an image/audio/file attachment. An
// explicit kind hint wins; otherwise the MIME is sniffed from the content.
func buildAttachment(data []byte, hint string) channel.Attachment {
	mime := http.DetectContentType(data)
	kind := hint
	if kind == "" {
		switch {
		case strings.HasPrefix(mime, "image/"):
			kind = "image"
		case strings.HasPrefix(mime, "audio/"), strings.HasPrefix(mime, "video/"):
			kind = "audio"
		default:
			kind = "file"
		}
	} else if mime == "application/octet-stream" {
		// Sniffing failed (common for audio formats DetectContentType can't read);
		// fall back to a sensible MIME for the hinted kind so the channel tags it.
		switch kind {
		case "image":
			mime = "image/png"
		case "audio":
			mime = "audio/ogg"
		}
	}
	return channel.Attachment{
		Kind:     kind,
		Data:     data,
		MIME:     mime,
		Filename: "media" + extForMIME(mime, kind),
	}
}

// extForMIME maps a MIME (or the kind, as a fallback) to a file extension.
func extForMIME(mime, kind string) string {
	switch {
	case strings.Contains(mime, "png"):
		return ".png"
	case strings.Contains(mime, "jpeg"), strings.Contains(mime, "jpg"):
		return ".jpg"
	case strings.Contains(mime, "gif"):
		return ".gif"
	case strings.Contains(mime, "webp"):
		return ".webp"
	case strings.Contains(mime, "ogg"), strings.Contains(mime, "opus"):
		return ".ogg"
	case strings.Contains(mime, "mpeg"), strings.Contains(mime, "mp3"):
		return ".mp3"
	case strings.Contains(mime, "wav"):
		return ".wav"
	case strings.Contains(mime, "pdf"):
		return ".pdf"
	}
	switch kind {
	case "image":
		return ".png"
	case "audio":
		return ".ogg"
	}
	return ".bin"
}
