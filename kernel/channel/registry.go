// SPDX-License-Identifier: MIT

package channel

import "sort"

// Manifest is a channel's self-description for the "systematik" channel layer:
// the metadata the Channels wizard renders (display name, what it is, transport,
// whether it's two-way) plus which Config Center section holds its account
// fields and which of those are required to consider it configured. Channels
// register a manifest so the console can list + configure them uniformly, and so
// a new gateway can be added by name (register a manifest + an account schema)
// without bespoke UI work.
type Manifest struct {
	Kind          string    `json:"kind"`           // stable id, e.g. "telegram" (matches Channel.Name())
	Display       string    `json:"display"`        // human label, e.g. "Telegram"
	Description   string    `json:"description"`    // one-line "what is this channel"
	Transport     string    `json:"transport"`      // "long-poll" | "webhook" | "rest" | "smtp"
	Duplex        bool      `json:"duplex"`         // true = two-way (can receive), false = outbound-only
	ConfigSection string    `json:"config_section"` // settings/Config Center section ID holding its fields
	RequiredEnv   []string  `json:"required_env"`   // env vars that must be set for the channel to start
	DocsURL       string    `json:"docs_url,omitempty"`
	Media         MediaCaps `json:"media"` // which non-text modalities this channel carries, per direction
}

// MediaCaps describes a channel's non-text multimodal reach. Text is always
// supported, so only image/voice are tracked, per direction. ImageOut/VoiceOut
// report whether the channel can deliver an outbound attachment of that kind.
type MediaCaps struct {
	ImageIn  bool `json:"image_in,omitempty"`
	VoiceIn  bool `json:"voice_in,omitempty"`
	ImageOut bool `json:"image_out,omitempty"`
	VoiceOut bool `json:"voice_out,omitempty"`
}

// registry is the process-wide set of registered channel manifests. The daemon
// seeds it (plugins/builtinchannels.RegisterAll); adding a channel = one more
// RegisterManifest call, no central edit.
var registry = map[string]Manifest{}

// RegisterManifest adds (or replaces) a channel manifest by kind. Idempotent.
func RegisterManifest(m Manifest) { registry[m.Kind] = m }

// Manifests returns all registered channel manifests, ordered by display name.
func Manifests() []Manifest {
	out := make([]Manifest, 0, len(registry))
	for _, m := range registry {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Display < out[j].Display })
	return out
}

// LookupManifest returns the manifest for a kind, if registered.
func LookupManifest(kind string) (Manifest, bool) {
	m, ok := registry[kind]
	return m, ok
}

// live is the set of channel kinds the daemon actually started this run. The
// daemon sets it after wiring its live channels; the Channels view reads it to
// show "live" vs merely "configured (restart to start)".
var live = map[string]bool{}

// SetLive records the channel kinds that are running this process. Replaces the
// prior set. Called once by the daemon after it builds its live channels.
func SetLive(kinds []string) {
	next := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		next[k] = true
	}
	live = next
}

// IsLive reports whether a channel kind is currently running.
func IsLive(kind string) bool { return live[kind] }
