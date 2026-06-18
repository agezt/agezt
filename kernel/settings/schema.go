// SPDX-License-Identifier: MIT

package settings

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// FieldType drives how the UI renders a field and how the server validates it.
type FieldType string

const (
	TypeText     FieldType = "text"
	TypePassword FieldType = "password" // secret; rendered masked, stored in the vault
	TypeNumber   FieldType = "number"
	TypeBool     FieldType = "bool"
	TypeCSV      FieldType = "csv" // comma-separated list (allowlists, recipients)
	TypeSelect   FieldType = "select"
)

// Apply says whether a change takes effect immediately or needs a restart. Only
// provider/model/catalog hot-reload today (via provider_reload); channels and
// interfaces are read once at startup.
type Apply string

const (
	ApplyLive    Apply = "live"
	ApplyRestart Apply = "restart"
)

// Field is one editable setting, keyed by its exact AGEZT_* env-var name.
type Field struct {
	Env      string    `json:"env"`
	Label    string    `json:"label"`
	Type     FieldType `json:"type"`
	Secret   bool      `json:"secret"` // true → stored in the vault, never echoed back
	Required bool      `json:"required"`
	Help     string    `json:"help,omitempty"`
	Apply    Apply     `json:"apply"`
	Options  []string  `json:"options,omitempty"` // for TypeSelect
	// ReadOnly: shown in the Config Center but NOT editable there (system-managed).
	// The server rejects any config_set for it; the UI renders it read-only.
	ReadOnly bool `json:"read_only,omitempty"`
	// Locked: the value may be changed but never CLEARED/removed ("silinemez").
	// The server rejects a config_set with an empty value; the UI hides Clear.
	Locked bool `json:"locked,omitempty"`
}

// Section groups related fields for the Config Center UI. Source records where
// the section came from — "builtin" for the compiled-in core config, or the
// registered schema's id for a skill/plugin-contributed section (see registry.go).
type Section struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Help   string `json:"help,omitempty"`
	Source string `json:"source,omitempty"`
	// Locked: a system-approved section that cannot be unregistered through the
	// normal path (config_schema_unregister / the `config` tool) — only with an
	// explicit operator force, or by deleting the file. Built-in sections are
	// always permanent regardless of this flag.
	Locked bool    `json:"locked,omitempty"`
	Fields []Field `json:"fields"`
}

// SourceBuiltin marks the compiled-in core configuration sections.
const SourceBuiltin = "builtin"

// Schema returns the built-in editable configuration surface. Kept for
// back-compat and as the seed for the Registry (registry.go), which merges
// these compiled-in sections with on-disk skill/plugin-registered ones.
func Schema() []Section {
	return builtinSections()
}

// builtinSections is the typed, grouped, secret-flagged description the Config
// Center renders forms from and the server validates against. Every Env here is
// an existing AGEZT_* var already consumed at startup, so editing it (via the
// config store / vault + startup injection) changes real behaviour. Each section
// is tagged Source = SourceBuiltin so the UI can tell core config apart from
// skill/plugin-registered sections.
func builtinSections() []Section {
	pw := func(env, label, help string) Field {
		return Field{Env: env, Label: label, Type: TypePassword, Secret: true, Apply: ApplyRestart, Help: help}
	}
	secs := []Section{
		{
			ID: "provider", Name: "Provider & Model",
			Help: "The active LLM provider and model. Applies live (the provider is rebuilt without a restart). There is no built-in default: blank provider = unconfigured (runs fail until set); model is resolved from routing/fallback chains when blank.",
			Fields: []Field{
				{Env: "AGEZT_PROVIDER", Label: "Provider", Type: TypeText, Apply: ApplyLive, Help: "Catalog provider id, e.g. deepseek, openai, anthropic. Required to dispatch LLM calls — blank leaves the daemon unconfigured."},
				{Env: "AGEZT_MODEL", Label: "Model", Type: TypeText, Apply: ApplyLive, Help: "Model id for runs. Blank = resolved per run from routing / a fallback chain; there is no built-in default model."},
			},
		},
		{
			ID: "embeddings", Name: "Memory Embeddings",
			Help: "Optional semantic embeddings for memory recall (M901). Unset = local hashing (free, typo-tolerant, no synonyms). Point at a local Ollama for free true-semantic recall, or a hosted API. Restart to apply.",
			Fields: []Field{
				{Env: "AGEZT_EMBED_URL", Label: "Embeddings API URL", Type: TypeText, Apply: ApplyRestart, Help: "OpenAI-compatible API root, e.g. http://localhost:11434 (Ollama) or https://api.openai.com/v1."},
				{Env: "AGEZT_EMBED_MODEL", Label: "Embedding model", Type: TypeText, Apply: ApplyRestart, Help: "e.g. nomic-embed-text (Ollama) or text-embedding-3-small (OpenAI)."},
				{Env: "AGEZT_EMBED_KEY", Label: "API key", Type: TypePassword, Secret: true, Apply: ApplyRestart, Help: "Bearer token for hosted APIs; leave empty for a local Ollama."},
			},
		},
		{
			ID: "telegram", Name: "Telegram",
			Help: "Telegram bot channel. Restart to apply.",
			Fields: []Field{
				pw("AGEZT_TELEGRAM_TOKEN", "Bot token", "From @BotFather."),
				{Env: "AGEZT_TELEGRAM_CHAT_ID", Label: "Allowed chat IDs", Type: TypeCSV, Apply: ApplyRestart, Help: "Comma-separated chat IDs allowed to talk to the agent and receive briefs."},
				{Env: "AGEZT_TELEGRAM_API_BASE", Label: "API base (optional)", Type: TypeText, Apply: ApplyRestart},
			},
		},
		{
			ID: "email", Name: "Email / SMTP",
			Help: "Outbound email channel. Restart to apply.",
			Fields: []Field{
				{Env: "AGEZT_EMAIL_SMTP_ADDR", Label: "SMTP host:port", Type: TypeText, Apply: ApplyRestart, Help: "e.g. smtp.gmail.com:587"},
				{Env: "AGEZT_EMAIL_FROM", Label: "From address", Type: TypeText, Apply: ApplyRestart},
				{Env: "AGEZT_EMAIL_USERNAME", Label: "SMTP username", Type: TypeText, Apply: ApplyRestart},
				pw("AGEZT_EMAIL_PASSWORD", "SMTP password", "Stored encrypted in the vault."),
				{Env: "AGEZT_EMAIL_RECIPIENTS", Label: "Allowed recipients", Type: TypeCSV, Apply: ApplyRestart},
			},
		},
		{
			ID: "slack", Name: "Slack",
			Help: "Slack bot channel. Restart to apply.",
			Fields: []Field{
				pw("AGEZT_SLACK_TOKEN", "Bot token", "xoxb-…"),
				pw("AGEZT_SLACK_SIGNING_SECRET", "Signing secret", "Required to verify inbound events."),
				{Env: "AGEZT_SLACK_CHANNELS", Label: "Allowed channels", Type: TypeCSV, Apply: ApplyRestart},
				{Env: "AGEZT_SLACK_ADDR", Label: "Inbound addr (optional)", Type: TypeText, Apply: ApplyRestart, Help: "host:port to serve /slack/events; blank = outbound only."},
			},
		},
		{
			ID: "discord", Name: "Discord",
			Help: "Discord bot channel. Restart to apply.",
			Fields: []Field{
				pw("AGEZT_DISCORD_TOKEN", "Bot token", ""),
				{Env: "AGEZT_DISCORD_APP_ID", Label: "App ID", Type: TypeText, Apply: ApplyRestart},
				pw("AGEZT_DISCORD_PUBLIC_KEY", "Public key", "For inbound webhook signature verification."),
				{Env: "AGEZT_DISCORD_CHANNELS", Label: "Allowed channels", Type: TypeCSV, Apply: ApplyRestart},
				{Env: "AGEZT_DISCORD_ADDR", Label: "Inbound addr (optional)", Type: TypeText, Apply: ApplyRestart},
			},
		},
		{
			ID: "interfaces", Name: "Interfaces",
			Help: "Network surfaces the daemon serves. Restart to apply.",
			Fields: []Field{
				{Env: "AGEZT_WEB_ADDR", Label: "Web UI addr", Type: TypeText, Apply: ApplyRestart, Help: "Where the console listens. Blank = on at 127.0.0.1:8787; set 'off' to disable."},
				{Env: "AGEZT_WEB_PASSWORD", Label: "Web UI password", Type: TypePassword, Secret: true, Apply: ApplyLive,
					Help: "Console password (M933): with it set you can open the console WITHOUT the URL token and log in here. Applies live. Blank = token-only."},
				{Env: "AGEZT_WEB_PASSWORD_STRICT", Label: "Password strict mode", Type: TypeBool, Apply: ApplyRestart,
					Help: "on = token AND password both required on every request (two factors) — for consoles exposed beyond loopback. Default: password OR token opens the console."},
				{Env: "AGEZT_API_ADDR", Label: "OpenAI-compatible API addr", Type: TypeText, Apply: ApplyRestart},
				{Env: "AGEZT_REST_ADDR", Label: "REST API addr", Type: TypeText, Apply: ApplyRestart},
			},
		},
		{
			ID: "limits", Name: "Budget & Limits",
			Help: "Rate and context budgets. Restart to apply.",
			Fields: []Field{
				{Env: "AGEZT_RATE_PER_MIN", Label: "Max requests / minute", Type: TypeNumber, Apply: ApplyRestart},
				{Env: "AGEZT_CONTEXT_BUDGET", Label: "Context budget (chars)", Type: TypeNumber, Apply: ApplyRestart},
				{Env: "AGEZT_OBSERVATION_DELTAS", Label: "Observation deltas", Type: TypeBool, Apply: ApplyRestart, Help: "on = repeated identical tool/input observations are shown to the model as deltas while raw output remains journaled."},
				{Env: "AGEZT_MAX_ITER", Label: "Max tool rounds / run", Type: TypeNumber, Apply: ApplyRestart, Help: "How many tool-call rounds one run may take before it stops (default 50). Chat can 'Continue' a run that hits the cap."},
				{Env: "AGEZT_PARALLEL_TOOLS", Label: "Parallel tools / turn", Type: TypeNumber, Apply: ApplyRestart, Help: "How many tool calls from one assistant turn may execute concurrently (default 4). 1 = strictly sequential."},
				{Env: "AGEZT_TOOL_DISCOVERY_MAX", Label: "Tool discovery max", Type: TypeNumber, Apply: ApplyRestart, Help: "Offer at most this many relevant tool schemas per model call using lexical discovery. Empty or 0 = offer all tools."},
				{Env: "AGEZT_DISABLE_HEURISTIC_BYPASS", Label: "Disable heuristic bypass", Type: TypeBool, Apply: ApplyRestart, Help: "on = route even simple deterministic date/time intents through the normal model loop."},
				{Env: "AGEZT_LLM_CACHE_TTL", Label: "LLM response cache TTL", Type: TypeText, Apply: ApplyRestart, Help: "Serve an IDENTICAL model request from memory within this window (e.g. 5m) — no provider call, no spend. Empty = off; chat regenerate wants fresh samples, so enable only for machine-driven repeat calls."},
			},
		},
		{
			ID: "alerts", Name: "Alert Notifications",
			Help: "Push warning/critical alerts (run failures, blocked egress, budget/rate trips, halts) to the configured channels. Restart to apply.",
			Fields: []Field{
				{Env: "AGEZT_ALERT_NOTIFY", Label: "Enabled", Type: TypeBool, Apply: ApplyRestart, Help: "Needs at least one configured channel (Telegram, Slack, …)."},
				{Env: "AGEZT_ALERT_NOTIFY_LEVEL", Label: "Minimum level", Type: TypeSelect, Apply: ApplyRestart, Options: []string{"", "warning", "critical"}, Help: "warning (default) sends warnings and criticals; critical sends criticals only."},
				{Env: "AGEZT_ALERT_NOTIFY_COOLDOWN", Label: "Repeat cooldown", Type: TypeText, Apply: ApplyRestart, Help: "The same alert (kind + run) is sent at most once per this window, e.g. 5m."},
				{Env: "AGEZT_ALERT_NOTIFY_MAX", Label: "Flood cap (per 10m)", Type: TypeNumber, Apply: ApplyRestart, Help: "Hard ceiling on notifications per 10-minute window; extra alerts are dropped."},
				{Env: "AGEZT_ALERT_NOTIFY_MUTE", Label: "Mute window", Type: TypeText, Apply: ApplyRestart, Help: "Daily quiet window in 24h START-END form, e.g. 0-7. Warnings are held; CRITICAL alerts (budget, halt) still break through."},
				{Env: "AGEZT_ALERT_NOTIFY_MUTE_SOURCES", Label: "Muted sources", Type: TypeText, Apply: ApplyRestart, Help: "Comma-separated categories to silence entirely (any level): run, egress, budget, provider, kernel."},
			},
		},
		{
			ID: "security", Name: "Security & Policy",
			Help: "Autonomy posture. Restart to apply.",
			Fields: []Field{
				{Env: "AGEZT_APPROVAL_MODE", Label: "Approval mode", Type: TypeSelect, Apply: ApplyRestart, Options: []string{"", "ask", "allow", "deny"}, Help: "How HITL approvals resolve by default."},
				{Env: "AGEZT_ALLOW_ALL", Label: "Allow all (DANGEROUS)", Type: TypeBool, Apply: ApplyRestart, Help: "Master permissive switch — grants every capability and opens the network tools."},
			},
		},
	}
	for i := range secs {
		secs[i].Source = SourceBuiltin
	}
	return secs
}

// FieldByEnv returns the schema field for an env-var name, and whether it's known.
func FieldByEnv(env string) (Field, bool) {
	for _, s := range Schema() {
		for _, f := range s.Fields {
			if f.Env == env {
				return f, true
			}
		}
	}
	return Field{}, false
}

// Validate checks a value against its field's type. Empty is always allowed
// (clearing a field). Returns nil for unknown fields the caller already rejected.
func Validate(f Field, value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	switch f.Type {
	case TypeNumber:
		if _, err := strconv.Atoi(v); err != nil {
			return fmt.Errorf("%s must be a whole number", f.Label)
		}
	case TypeBool:
		switch strings.ToLower(v) {
		case "1", "0", "true", "false", "on", "off", "yes", "no":
		default:
			return fmt.Errorf("%s must be a boolean (on/off, true/false, 1/0)", f.Label)
		}
	case TypeSelect:
		if !slices.Contains(f.Options, v) {
			return fmt.Errorf("%s must be one of: %s", f.Label, strings.Join(f.Options, ", "))
		}
	}
	return nil
}
