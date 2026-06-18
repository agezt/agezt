// Curated provider presets for the "Quick Connect" gallery — paste a key and
// the provider is registered (custom.json) + keyed in one step. Each preset
// carries everything the daemon needs: catalog id, wire family, base URL, the
// key env var, and a default model. Base URL + model stay editable in the UI,
// so a best-effort endpoint (MiMo, opencode) is never a hard block.
//
// AFFILIATE LINKS: every signup/docs URL lives in THIS file. To monetize, swap
// each `signupUrl` for its affiliate/ref variant here — nothing else changes.

export type PresetFamily = "openai-compatible" | "anthropic";

export interface ProviderPreset {
  /** Stable catalog id (also the custom.json key). */
  id: string;
  /** Human label shown on the card. */
  name: string;
  /** Vendor group key (so OpenAI + Anthropic cards cluster). */
  vendor: string;
  /** Short badge, e.g. "Coding plan", "Token plan", "API". */
  tagline: string;
  category: "coding" | "popular";
  family: PresetFamily;
  /** Base URL (editable in the card). */
  api: string;
  /** Key env var the keyring stores under (UPPER_SNAKE, not AGEZT_*). */
  keyEnv: string;
  /** Default model id (editable in the card). */
  model: string;
  /** Brand accent color + glyph for the card. */
  color: string;
  glyph: string;
  /** Where to get a key — affiliate-swappable. */
  signupUrl: string;
  docsUrl: string;
}

/** Maps a preset family to the catalog NPM wire-family hint. */
export function familyNpm(f: PresetFamily): string {
  return f === "anthropic" ? "@ai-sdk/anthropic" : "@ai-sdk/openai-compatible";
}

/** A short label for the compatibility family, shown on the card. */
export function familyLabel(f: PresetFamily): string {
  return f === "anthropic" ? "Anthropic-compatible" : "OpenAI-compatible";
}

export const PROVIDER_PRESETS: ProviderPreset[] = [
  // ---- Coding / token plans (the headline gallery) ----
  {
    id: "zai-coding", name: "Z.ai GLM", vendor: "zai", tagline: "Coding plan", category: "coding",
    family: "openai-compatible", api: "https://api.z.ai/api/coding/paas/v4", keyEnv: "ZAI_API_KEY", model: "glm-4.6",
    color: "#2563eb", glyph: "Z", signupUrl: "https://z.ai/manage-apikey/apikey-list", docsUrl: "https://docs.z.ai/",
  },
  {
    id: "zai-anthropic", name: "Z.ai GLM", vendor: "zai", tagline: "Coding · Claude-style", category: "coding",
    family: "anthropic", api: "https://api.z.ai/api/anthropic", keyEnv: "ZAI_API_KEY", model: "glm-4.6",
    color: "#2563eb", glyph: "Z", signupUrl: "https://z.ai/manage-apikey/apikey-list", docsUrl: "https://docs.z.ai/",
  },
  {
    id: "minimax", name: "MiniMax", vendor: "minimax", tagline: "Coding / token plan", category: "coding",
    family: "openai-compatible", api: "https://api.minimax.io/v1", keyEnv: "MINIMAX_API_KEY", model: "MiniMax-M2",
    color: "#e11d48", glyph: "M", signupUrl: "https://platform.minimax.io/", docsUrl: "https://platform.minimax.io/docs",
  },
  {
    id: "minimax-anthropic", name: "MiniMax", vendor: "minimax", tagline: "Claude-style", category: "coding",
    family: "anthropic", api: "https://api.minimax.io/anthropic", keyEnv: "MINIMAX_API_KEY", model: "MiniMax-M2",
    color: "#e11d48", glyph: "M", signupUrl: "https://platform.minimax.io/", docsUrl: "https://platform.minimax.io/docs",
  },
  {
    id: "moonshot", name: "Kimi (Moonshot)", vendor: "moonshot", tagline: "Coding plan", category: "coding",
    family: "openai-compatible", api: "https://api.moonshot.ai/v1", keyEnv: "MOONSHOT_API_KEY", model: "kimi-k2-0711-preview",
    color: "#7c3aed", glyph: "K", signupUrl: "https://platform.moonshot.ai/console/api-keys", docsUrl: "https://platform.moonshot.ai/docs",
  },
  {
    id: "moonshot-anthropic", name: "Kimi (Moonshot)", vendor: "moonshot", tagline: "Claude-style", category: "coding",
    family: "anthropic", api: "https://api.moonshot.ai/anthropic", keyEnv: "MOONSHOT_API_KEY", model: "kimi-k2-0711-preview",
    color: "#7c3aed", glyph: "K", signupUrl: "https://platform.moonshot.ai/console/api-keys", docsUrl: "https://platform.moonshot.ai/docs",
  },
  {
    id: "deepseek", name: "DeepSeek", vendor: "deepseek", tagline: "API", category: "coding",
    family: "openai-compatible", api: "https://api.deepseek.com", keyEnv: "DEEPSEEK_API_KEY", model: "deepseek-chat",
    color: "#4f46e5", glyph: "D", signupUrl: "https://platform.deepseek.com/api_keys", docsUrl: "https://api-docs.deepseek.com/",
  },
  {
    id: "deepseek-anthropic", name: "DeepSeek", vendor: "deepseek", tagline: "Claude-style", category: "coding",
    family: "anthropic", api: "https://api.deepseek.com/anthropic", keyEnv: "DEEPSEEK_API_KEY", model: "deepseek-chat",
    color: "#4f46e5", glyph: "D", signupUrl: "https://platform.deepseek.com/api_keys", docsUrl: "https://api-docs.deepseek.com/",
  },
  {
    id: "mimo", name: "MiMo (Xiaomi)", vendor: "mimo", tagline: "Token plan", category: "coding",
    family: "openai-compatible", api: "https://api.mimo.ai/v1", keyEnv: "MIMO_API_KEY", model: "mimo-7b-rl",
    color: "#f97316", glyph: "Mi", signupUrl: "https://xiaomimimo.com/", docsUrl: "https://github.com/XiaomiMiMo",
  },
  {
    id: "opencode", name: "opencode zen", vendor: "opencode", tagline: "Gateway", category: "coding",
    family: "openai-compatible", api: "https://opencode.ai/zen/v1", keyEnv: "OPENCODE_API_KEY", model: "claude-sonnet-4-5",
    color: "#0ea5e9", glyph: "OC", signupUrl: "https://opencode.ai/", docsUrl: "https://opencode.ai/docs/zen/",
  },

  // ---- Popular extras (OpenAI-compatible, low-friction) ----
  {
    id: "openrouter", name: "OpenRouter", vendor: "openrouter", tagline: "300+ models", category: "popular",
    family: "openai-compatible", api: "https://openrouter.ai/api/v1", keyEnv: "OPENROUTER_API_KEY", model: "deepseek/deepseek-chat",
    color: "#6366f1", glyph: "OR", signupUrl: "https://openrouter.ai/keys", docsUrl: "https://openrouter.ai/docs",
  },
  {
    id: "groq", name: "Groq", vendor: "groq", tagline: "Fast inference", category: "popular",
    family: "openai-compatible", api: "https://api.groq.com/openai/v1", keyEnv: "GROQ_API_KEY", model: "llama-3.3-70b-versatile",
    color: "#f55036", glyph: "G", signupUrl: "https://console.groq.com/keys", docsUrl: "https://console.groq.com/docs",
  },
  {
    id: "cerebras", name: "Cerebras", vendor: "cerebras", tagline: "Fastest tokens", category: "popular",
    family: "openai-compatible", api: "https://api.cerebras.ai/v1", keyEnv: "CEREBRAS_API_KEY", model: "llama-3.3-70b",
    color: "#f59e0b", glyph: "C", signupUrl: "https://cloud.cerebras.ai/", docsUrl: "https://inference-docs.cerebras.ai/",
  },
  {
    id: "together", name: "Together AI", vendor: "together", tagline: "Open models", category: "popular",
    family: "openai-compatible", api: "https://api.together.xyz/v1", keyEnv: "TOGETHER_API_KEY", model: "deepseek-ai/DeepSeek-V3",
    color: "#0f766e", glyph: "T", signupUrl: "https://api.together.ai/settings/api-keys", docsUrl: "https://docs.together.ai/",
  },
  {
    id: "fireworks", name: "Fireworks", vendor: "fireworks", tagline: "Open models", category: "popular",
    family: "openai-compatible", api: "https://api.fireworks.ai/inference/v1", keyEnv: "FIREWORKS_API_KEY", model: "accounts/fireworks/models/deepseek-v3",
    color: "#db2777", glyph: "F", signupUrl: "https://fireworks.ai/account/api-keys", docsUrl: "https://docs.fireworks.ai/",
  },
  {
    id: "mistral", name: "Mistral", vendor: "mistral", tagline: "API", category: "popular",
    family: "openai-compatible", api: "https://api.mistral.ai/v1", keyEnv: "MISTRAL_API_KEY", model: "mistral-large-latest",
    color: "#ea580c", glyph: "Mi", signupUrl: "https://console.mistral.ai/api-keys/", docsUrl: "https://docs.mistral.ai/",
  },
  {
    id: "xai", name: "xAI Grok", vendor: "xai", tagline: "API", category: "popular",
    family: "openai-compatible", api: "https://api.x.ai/v1", keyEnv: "XAI_API_KEY", model: "grok-2-latest",
    color: "#111827", glyph: "X", signupUrl: "https://console.x.ai/", docsUrl: "https://docs.x.ai/",
  },
];
