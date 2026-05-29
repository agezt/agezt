# Agezt — Widget System & SDK Specification (SPEC-12)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> Depends on: SPEC-07 (UI), SPEC-08 (plugin contributions), SPEC-06 (security)
> Defines rich, interactive in-conversation widgets and the SDK to build them — turning chat from text into a live, interactive surface. Widgets are a UI contribution, parallel to how tools are a capability contribution.

---

## 1. Concept

An agent should be able to reply with more than text. A **widget** is a rich, interactive UI component rendered inline in the conversation: a chart, table, approval button, form, map, file preview, or even a live mini React Flow DAG. Just as a `ToolPlugin` adds a capability to the kernel, a **widget** adds a rendering to the chat surface — declared as a plugin contribution (SPEC-08 §1, `ui_widgets`).

This is the natural endpoint of "decorate conversations with widgets (even an SDK)": widgets are not hardcoded; they're an open, SDK-driven extension point.

---

## 2. Widget model

```ts
// widget contract (agezt-sdk-ts / widget submodule)
interface AgeztWidget<Data, Action = unknown> {
  kind: string;                 // "chart.bar", "form.approval", "map", "flow.mini", ...
  schema: JSONSchema;           // the Data shape the widget accepts
  render(data: Data, ctx: WidgetContext): ReactNode;
  // optional: emit an event back to the kernel (e.g. an approval, a form submit)
  onAction?(action: Action, ctx: WidgetContext): void; // → kernel event
}
```

- A tool/agent returns a **widget descriptor** alongside (or instead of) text: `{ widget: "chart.bar", data: {...} }`. The UI looks up the registered widget for that `kind` and renders it.
- Widgets can be **interactive**: a button/form emits an event (e.g. `EVT_APPROVAL_GRANTED`, or a custom `EVT_WIDGET_ACTION`) back through the gateway → kernel, closing the loop with the running agent.
- Widgets are **versioned and content-addressed** (like plugins/skills); distributable via the marketplace.

---

## 3. Security (this is the critical part)

In-conversation interactive UI is a surface where prompt-injection could leak into the UI layer. Therefore:

- **Widgets render data, they do not execute arbitrary code.** A widget is a registered component with a fixed `kind`; the agent supplies *data*, not code. The agent cannot inject a new executable component at runtime — only choose among registered widget kinds and pass schema-validated data.
- **Sandboxed rendering:** widgets render in an isolated context (iframe/shadow-DOM with a strict CSP); they cannot access the host page, cookies, tokens, or other widgets. (Aligns with SPEC-06.)
- **Schema-validated data:** widget input is validated against its `schema` before render; malformed/oversized data is rejected.
- **Capability-scoped actions:** a widget's `onAction` can only emit events the widget's plugin is authorized for (Edict); an "approve" widget can't quietly trigger a purchase.
- **Untrusted content stays data:** content arriving from channels/web that *contains* a widget descriptor is treated with the same caution — it cannot escalate by rendering.

Honest note: this is genuinely the riskiest UI feature; the "data not code" + sandbox + capability-scoping triad is non-negotiable. Widgets that need real logic must do it server-side (in their plugin), not in the rendered component.

---

## 4. First-party widgets

- **Data:** table, bar/line/area chart, stat/metric, JSON tree, diff viewer.
- **Interactive:** approval (approve/deny/scope), form (schema-driven input → event), choice/clarify (the human-in-the-loop "did you mean X or Y?" — SPEC-14).
- **Spatial:** map (places), timeline, mini React Flow (a live sub-DAG of the current task).
- **Media/artifacts:** image preview, file/artifact card (download, open), audio player.
- **System:** tool-call card (expandable: input/output/isolation/policy/cost — the debug view, SPEC-07), context-inspector card (what was in the LLM's context).

These ship with the Web UI; third parties add more via the SDK.

---

## 5. SDK & developer experience

- `agezt-sdk-ts` includes a **widget submodule**: define a widget by implementing the contract; register its `kind`; ship it as part of a plugin's `ui_widgets` contribution.
- `create-agezt-plugin` scaffolds a plugin with an optional widget.
- Widgets are typed end-to-end: the `schema` generates a TS type for `Data`, so the plugin's tool output and the widget input are type-checked against each other.
- Marketplace: "install this nicer chart widget; route your tool outputs to render with it."

---

## 6. Integration with conversation surface (SPEC-07)

The conversation/chat surface renders an agent turn as an ordered list of blocks: text, widgets, and tool-call cards interleaved — exactly the "widget-decorated conversations" vision. Each block is journal-backed (reproducible, exportable). The same widgets work in the Web UI; channels that support rich content (Telegram inline keyboards, etc.) get a graceful mapping, and plain channels fall back to text + a link.

---

## 7. Phase placement

- Widget contract + first-party data/approval/tool-call/context-inspector widgets + sandboxed render: **Phase 5** (with the Web UI conversation surface).
- Widget SDK submodule + scaffolder + typed schema↔data: **Phase 7** (with full SDKs).
- Marketplace widgets: **Phase 8**.

---

## 8. Open questions

1. Render isolation mechanism: iframe vs shadow-DOM + CSP — which gives the right security/UX balance.
2. How rich can interactive widgets be before they need their own state/backchannel beyond single actions.
3. Channel mapping fidelity: how much widget richness survives on Telegram/Discord vs graceful text fallback.
4. Versioning/compat when a widget's schema evolves but old conversations reference the old shape.

---

*Next: SPEC-13 (Capability Army) and SPEC-14 (Resilience, HITL, Eval, RBAC, Onboarding).*
