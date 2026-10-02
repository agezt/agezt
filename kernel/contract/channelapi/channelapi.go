// SPDX-License-Identifier: MIT

// Package channelapi is the messaging-channel contract: the platform-neutral
// inbound message every channel normalises to (UnifiedMessage, SPEC-04 §1.3),
// what the kernel hands a channel to deliver (Outbound, Attachment, Reply), the
// Channel interface a duplex surface implements, and the Manifest a channel
// registers to describe itself. Pure types — no logic, standard library only —
// so any layer may depend on it (architecture/20-target-architecture.md §2,
// layer L1).
//
// kernel/channel used to hold these next to process-global registry state and
// a bus-publishing panic guard, which pulled the event bus into what should be
// a leaf. kernel/channel keeps type aliases, so existing importers are
// unaffected.
package channelapi

import "context"

// UnifiedMessage is the platform-neutral inbound message (SPEC-04 §1.3,
// mirrors .project/agezt.proto §UnifiedMessage). Native concepts map on:
// conversation/thread → ChannelID; body → Text; anything platform-specific is
// preserved in PlatformMeta (never lost, not required by core).
type UnifiedMessage struct {
	ChannelKind string `json:"channel_kind"` // "telegram", "discord", ...
	ChannelID   string `json:"channel_id"`   // conversation/chat id
	// ThreadID is the platform thread/topic WITHIN the channel (M885): Slack
	// thread_ts, Telegram forum topic id. Empty = the channel's main stream.
	// Platforms whose threads are first-class channels (Discord) leave it
	// empty — their thread already IS the ChannelID. Conversation history is
	// folded per (channel, thread), so two threads in one channel are two
	// separate conversations, and replies land back in the same thread.
	ThreadID     string            `json:"thread_id,omitempty"`
	Sender       string            `json:"sender"`                   // platform user id/handle
	Text         string            `json:"text"`                     // message body
	Images       []string          `json:"images,omitempty"`         // inbound image attachments as data: URLs (M247)
	Audio        []string          `json:"audio,omitempty"`          // inbound audio attachments (voice notes) as data: URLs — auto-transcribed when STT is configured
	PlatformTSMS int64             `json:"platform_ts_ms,omitempty"` // platform timestamp
	PlatformMeta map[string]string `json:"platform_meta,omitempty"`  // preserved extras
}

// Priority lets the Briefing composer drive delivery urgency (SPEC-04 §1.5).
type Priority string

const (
	PriorityInfo   Priority = "info"
	PriorityNotify Priority = "notify"
	PriorityUrgent Priority = "urgent"
)

// Attachment is one piece of outbound media (image, audio/voice, or file) the
// agent wants delivered alongside (or instead of) text. Data carries the raw
// bytes — the daemon resolves any artifact reference to bytes before handing it
// to a channel, so channel packages never import the artifact store. Channels
// whose platform has no media API ignore Attachments (text still delivers).
type Attachment struct {
	Kind     string `json:"kind"`               // "image" | "audio" | "file"
	Data     []byte `json:"-"`                  // raw bytes (not serialized in events)
	MIME     string `json:"mime,omitempty"`     // e.g. "image/png", "audio/ogg"
	Filename string `json:"filename,omitempty"` // suggested filename
}

// Outbound is a message the kernel hands a channel to deliver.
type Outbound struct {
	ChannelID string `json:"channel_id"`
	// ThreadID targets a thread/topic within the channel (M885) — the reply
	// half of UnifiedMessage.ThreadID. Empty posts to the main stream.
	ThreadID string   `json:"thread_id,omitempty"`
	Text     string   `json:"text"`
	Priority Priority `json:"priority,omitempty"`
	// Attachments are outbound media (images / voice clips / files). Channels
	// that support media send them; text-only channels ignore the slice.
	Attachments []Attachment `json:"-"`
}

// Reply is what an InboundHandler returns: the text answer plus any media the
// agent produced (a synthesized voice clip for a voice message, a generated
// image, …). Text-only replies set just Text; an empty Text with no Attachments
// means "nothing to send back".
type Reply struct {
	Text        string
	Attachments []Attachment
}

// InboundHandler turns an inbound message into a reply. The daemon supplies it
// (wired to the agent loop). corr is the correlation the channel minted for
// this exchange, so the handler can run the agent under it and keep the
// channel.inbound/outbound events linked to the agent's task arc. An empty
// Reply (no text, no attachments) means "nothing to send back". A non-nil error
// is surfaced to the user as a short failure notice.
type InboundHandler func(ctx context.Context, msg UnifiedMessage, corr string) (Reply, error)

// Channel is a duplex messaging surface (SPEC-04 §1.2).
type Channel interface {
	// Name identifies the channel kind ("telegram").
	Name() string
	// Start begins listening; inbound messages flow to the handler. Returns
	// when ctx is cancelled (or on a fatal connect error).
	Start(ctx context.Context) error
	// Send delivers an outbound message.
	Send(ctx context.Context, out Outbound) error
}

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
	// SetupSteps are terse "what you'll need / how to get credentials" bullets the
	// guided Connect flow shows before the field form. Optional.
	SetupSteps []string `json:"setup_steps,omitempty"`
	// ConnectMethod tells the UI how to connect: "token" (default — paste fields),
	// "qr" (scan, e.g. whatsappgw), "gateway" (self-hosted URL + reachability), or
	// "oauth" (authorize in browser). Empty = "token".
	ConnectMethod string `json:"connect_method,omitempty"`

	// The three fields below drive derived STATUS reporting (`agt status`), so
	// the daemon never needs a hand-maintained per-kind predicate list (which
	// had silently drifted to cover 11 of the registered kinds). All optional.
	//
	// AddrEnv names the env var holding the channel's serve/endpoint address.
	AddrEnv string `json:"addr_env,omitempty"`
	// AllowlistEnv names the env var whose comma-separated value is the
	// channel's recipient/room/channel allowlist.
	AllowlistEnv string `json:"allowlist_env,omitempty"`
	// InboundEnv lists env vars that must ALSO be set (beyond RequiredEnv) for
	// the channel to actually receive inbound traffic. Meaningful only for
	// Duplex channels; empty means inbound whenever configured.
	InboundEnv []string `json:"inbound_env,omitempty"`

	// BannerLabel is the short label the daemon's boot banner prints for this
	// channel (e.g. "webhook channel", "ntfy push"). Empty → Kind is used.
	BannerLabel string `json:"banner_label,omitempty"`
	// DisabledHint is the boot-banner line printed when no instance of this
	// channel is configured (e.g. "disabled (set AGEZT_TELEGRAM_TOKEN)").
	// Empty → the channel stays silent when unconfigured.
	DisabledHint string `json:"disabled_hint,omitempty"`
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
