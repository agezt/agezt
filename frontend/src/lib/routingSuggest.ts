// Auto-fill for the Routing view (M928): build a suggested per-task model chain
// from every KEYED (credentialed) provider in the catalog, so an operator who
// just added their API keys gets a working multi-provider routing table in one
// click — then edits instead of assembling each chain by hand.
//
// Shape of a suggestion: ONE best model per provider (diversity — a provider
// outage falls over to a DIFFERENT provider, not a sibling model on the same
// down endpoint), ordered by task fit, capped at SUGGEST_CHAIN_MAX entries.
// "Task fit" uses the only signals the catalog carries: tool_call, reasoning,
// context window and price. Output price is the strength proxy — heavy tasks
// (chat/plan/code/…) take the strongest model per provider, light tasks
// (summarize/salience/…) the cheapest non-reasoning one.

import { flattenModels, type ModelCatalog, type ModelOption } from "./models";

export const SUGGEST_CHAIN_MAX = 5;

// Models that can't serve a text completion chain at all (embeddings, speech,
// image generation…). Matched against the model id + name.
const NON_CHAT = /embed|tts|image|audio|whisper|transcribe|moderation|rerank|dall-e/i;

export interface TaskProfile {
  kind: "heavy" | "light";
  // heavy only: prefer reasoning-capable models (plan).
  preferReasoning?: boolean;
  // heavy only: the task drives tools, so tool-capable models are required
  // when the provider has any.
  requireTools?: boolean;
}

// One profile per known task type; unknown/custom task types fall back to a
// plain heavy profile.
export const TASK_PROFILES: Record<string, TaskProfile> = {
  chat: { kind: "heavy", requireTools: true },
  code: { kind: "heavy", requireTools: true },
  delegate: { kind: "heavy", requireTools: true },
  forge: { kind: "heavy", requireTools: true },
  plan: { kind: "heavy", preferReasoning: true },
  verify: { kind: "heavy" },
  summarize: { kind: "light" },
  salience: { kind: "light" },
  distill: { kind: "light" },
  "shadow-eval": { kind: "light" },
};

// compare orders two candidates by task fit (best first).
function compare(a: ModelOption, b: ModelOption, p: TaskProfile): number {
  if (p.kind === "heavy") {
    if (p.preferReasoning && a.reasoning !== b.reasoning) return a.reasoning ? -1 : 1;
    if (a.costOutput !== b.costOutput) return b.costOutput - a.costOutput; // stronger first
    return b.context - a.context;
  }
  if (a.costOutput !== b.costOutput) return a.costOutput - b.costOutput; // cheaper first
  return b.context - a.context;
}

// pickBest returns one provider's best model for the profile, or undefined.
function pickBest(models: ModelOption[], p: TaskProfile): ModelOption | undefined {
  let pool = models;
  if (p.requireTools) {
    const tools = pool.filter((o) => o.toolCall);
    if (tools.length) pool = tools;
  }
  if (p.kind === "light") {
    // Light jobs run constantly — avoid slow thinking models when the
    // provider offers anything else.
    const plain = pool.filter((o) => !o.reasoning);
    if (plain.length) pool = plain;
  }
  return [...pool].sort((a, b) => compare(a, b, p))[0];
}

// suggestChains builds {task: [model, …]} for every given task type from the
// catalog's credentialed providers. Returns an empty object when no keyed
// provider has a usable model.
export function suggestChains(cat: ModelCatalog | null | undefined, taskTypes: string[]): Record<string, string[]> {
  const usable = flattenModels(cat).filter(
    (o) => o.credentialed && !NON_CHAT.test(o.id) && !NON_CHAT.test(o.name),
  );
  const byProvider = new Map<string, ModelOption[]>();
  for (const o of usable) {
    const cur = byProvider.get(o.providerId);
    if (cur) cur.push(o);
    else byProvider.set(o.providerId, [o]);
  }

  const out: Record<string, string[]> = {};
  for (const task of taskTypes) {
    const profile = TASK_PROFILES[task] ?? { kind: "heavy" as const };
    const picks: ModelOption[] = [];
    for (const models of byProvider.values()) {
      const best = pickBest(models, profile);
      if (best) picks.push(best);
    }
    picks.sort((a, b) => compare(a, b, profile));
    const chain: string[] = [];
    for (const o of picks) {
      // Provider aliases (e.g. minimax / minimax-cn) often serve the same
      // model id under the same key — one chain entry is enough.
      if (!chain.includes(o.id)) chain.push(o.id);
      if (chain.length >= SUGGEST_CHAIN_MAX) break;
    }
    if (chain.length) out[task] = chain;
  }
  return out;
}
