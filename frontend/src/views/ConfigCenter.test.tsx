// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

// Mock the api layer so the view's fetches are deterministic.
const getJSON = vi.fn();
const postJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));

import { ConfigCenter, agentConfigScopeLabel, summarizeAgentConfigEntries } from "@/views/ConfigCenter";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

const SCHEMA = {
  sections: [
    {
      id: "provider",
      name: "Provider & Model",
      help: "Applies live.",
      source: "builtin",
      fields: [
        { env: "AGEZT_PROVIDER", label: "Provider", type: "text", secret: false, required: false, apply: "live" },
        { env: "AGEZT_MODEL", label: "Model", type: "text", secret: false, required: false, apply: "live" },
      ],
    },
    {
      id: "telegram",
      name: "Telegram",
      source: "builtin",
      fields: [
        { env: "AGEZT_TELEGRAM_TOKEN", label: "Bot token", type: "password", secret: true, required: false, apply: "restart" },
        { env: "AGEZT_TELEGRAM_CHAT_ID", label: "Allowed chat IDs", type: "csv", secret: false, required: false, apply: "restart" },
      ],
    },
    {
      id: "weather-skill",
      name: "Weather Skill",
      source: "weather-skill",
      fields: [{ env: "AGEZT_X_WEATHER_UNITS", label: "Units", type: "text", secret: false, required: false, apply: "restart" }],
    },
  ],
};

function valuesPayload(over: Record<string, any> = {}) {
  return {
    fields: [
      { env: "AGEZT_PROVIDER", secret: false, env_pinned: false, set: false, value: "", ...over.AGEZT_PROVIDER },
      { env: "AGEZT_MODEL", secret: false, env_pinned: false, set: true, value: "deepseek-chat", ...over.AGEZT_MODEL },
      { env: "AGEZT_TELEGRAM_TOKEN", secret: true, env_pinned: false, set: true, ...over.AGEZT_TELEGRAM_TOKEN },
      { env: "AGEZT_TELEGRAM_CHAT_ID", secret: false, env_pinned: false, set: false, value: "", ...over.AGEZT_TELEGRAM_CHAT_ID },
      { env: "AGEZT_X_WEATHER_UNITS", secret: false, env_pinned: false, set: false, value: "", ...over.AGEZT_X_WEATHER_UNITS },
    ],
  };
}

function mockFetch(values = valuesPayload()) {
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/config/schema") return Promise.resolve(SCHEMA);
    if (path === "/api/config/values") return Promise.resolve(values);
    if (path === "/api/configcenter/list") return Promise.resolve({ entries: [] });
    return Promise.reject(new Error("unexpected " + path));
  });
}

// Section names appear in BOTH the sticky nav (buttons) and the section cards
// (headings); assert on the heading so the query is unambiguous.
const sectionHeading = (name: string) => screen.getByRole("heading", { name });

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
});

describe("ConfigCenter view", () => {
  it("summarizes agent config scope and sensitivity", () => {
    expect(agentConfigScopeLabel({ key: "agent/ops/runtime", allowed_agents: ["ops"] })).toBe("identity-bound");
    expect(agentConfigScopeLabel({ key: "shared/api-key", excluded_agents: ["ops"] })).toBe("shared with denylist");
    expect(agentConfigScopeLabel({ key: "agent/ops/runtime" })).toBe("agent namespace");
    expect(agentConfigScopeLabel({ key: "global/runtime" })).toBe("shared");
    expect(
      summarizeAgentConfigEntries([
        { key: "agent/ops/runtime", allowed_agents: ["ops"], rating: "internal" },
        { key: "shared/api-key", rating: "secret" },
        { key: "agent/planner/runtime", rating: "restricted" },
      ]),
    ).toEqual({ total: 3, identityBound: 1, shared: 2, restricted: 1, secret: 1 });
  });

  it("renders sections grouped with a field input from the schema", async () => {
    mockFetch();
    render(withUI(<ConfigCenter />));
    await waitFor(() => expect(sectionHeading("Provider & Model")).toBeTruthy());
    expect(sectionHeading("Agent Runtime Config")).toBeTruthy();
    expect(sectionHeading("Telegram")).toBeTruthy();
    // Category headings exist.
    expect(sectionHeading("Core")).toBeTruthy();
    expect(sectionHeading("Channels")).toBeTruthy();
    // Non-secret value is pre-filled.
    expect((screen.getByDisplayValue("deepseek-chat") as HTMLInputElement).value).toBe("deepseek-chat");
  });

  it("groups a registered section under Skills & Plugins with a provenance badge", async () => {
    mockFetch();
    render(withUI(<ConfigCenter />));
    await waitFor(() => expect(sectionHeading("Weather Skill")).toBeTruthy());
    expect(sectionHeading("Skills & Plugins")).toBeTruthy();
    expect(screen.getByText("registered")).toBeTruthy();
  });

  it("filters fields by the search query", async () => {
    mockFetch();
    render(withUI(<ConfigCenter />));
    await waitFor(() => expect(sectionHeading("Telegram")).toBeTruthy());

    fireEvent.change(screen.getByLabelText("Search settings"), { target: { value: "weather" } });

    // Only the matching section survives; the others drop out.
    await waitFor(() => expect(screen.queryByRole("heading", { name: "Telegram" })).toBeNull());
    expect(sectionHeading("Weather Skill")).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Provider & Model" })).toBeNull();
  });

  it("shows an empty state when nothing matches the search", async () => {
    mockFetch();
    render(withUI(<ConfigCenter />));
    await waitFor(() => expect(sectionHeading("Telegram")).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Search settings"), { target: { value: "zzz-nomatch" } });
    await waitFor(() => expect(screen.getByText("No settings match")).toBeTruthy());
  });

  it("renders read-only fields non-editable and locked fields without a Clear button", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/config/schema")
        return Promise.resolve({
          sections: [
            {
              id: "sys",
              name: "System Managed",
              source: "sys",
              fields: [
                { env: "AGEZT_X_RO", label: "Read only", type: "text", secret: false, required: false, apply: "restart", read_only: true },
                { env: "AGEZT_X_LOCKED", label: "Locked secret", type: "password", secret: true, required: false, apply: "restart", locked: true },
              ],
            },
          ],
        });
      if (path === "/api/config/values")
        return Promise.resolve({
          fields: [
            { env: "AGEZT_X_RO", secret: false, env_pinned: false, set: true, value: "system-value" },
            { env: "AGEZT_X_LOCKED", secret: true, env_pinned: false, set: true },
          ],
        });
      if (path === "/api/configcenter/list") return Promise.resolve({ entries: [] });
      return Promise.reject(new Error("unexpected " + path));
    });
    render(withUI(<ConfigCenter />));
    await waitFor(() => expect(sectionHeading("System Managed")).toBeTruthy());

    // Read-only: value displayed, "read-only" chip, but NOT an editable input.
    expect(screen.getByText("read-only")).toBeTruthy();
    expect(screen.getByText("system-value")).toBeTruthy();
    expect(screen.queryByDisplayValue("system-value")).toBeNull();

    // Locked secret: a "locked" chip and no Clear button.
    expect(screen.getByText("locked")).toBeTruthy();
    expect(screen.queryByTitle("Clear (remove from vault)")).toBeNull();
  });

  it("never shows a secret value — only presence", async () => {
    mockFetch();
    const { container } = render(withUI(<ConfigCenter />));
    await waitFor(() => expect(sectionHeading("Telegram")).toBeTruthy());
    const pw = container.querySelector('input[type="password"]') as HTMLInputElement;
    expect(pw).toBeTruthy();
    expect(pw.value).toBe("");
    expect(screen.getByText(/set — type a new value to replace/)).toBeTruthy();
  });

  it("saves a non-secret field and reports applied-live", async () => {
    mockFetch();
    postJSON.mockResolvedValueOnce({ env: "AGEZT_MODEL", saved: true, applied: "live" });
    render(withUI(<ConfigCenter />));
    await waitFor(() => expect(screen.getByDisplayValue("deepseek-chat")).toBeTruthy());

    const input = screen.getByDisplayValue("deepseek-chat") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "deepseek-reasoner" } });
    const save = input.parentElement?.querySelector('button[title="Save"]') as HTMLButtonElement;
    fireEvent.click(save);

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_MODEL", value: "deepseek-reasoner" }),
    );
    await waitFor(() => expect(screen.getByText("Model applied live")).toBeTruthy());
  });

  it("saves an agent runtime config entry with allow and deny lists", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/config/schema") return Promise.resolve(SCHEMA);
      if (path === "/api/config/values") return Promise.resolve(valuesPayload());
      if (path === "/api/configcenter/list")
        return Promise.resolve({
          entries: [{ key: "agent/ops/runtime", value: "mode=careful", rating: "internal", allowed_agents: ["ops"], excluded_agents: ["blocked"] }],
        });
      return Promise.reject(new Error("unexpected " + path));
    });
    postJSON.mockResolvedValueOnce({ entry: { key: "agent/ops/runtime" } });
    render(withUI(<ConfigCenter />));
    await waitFor(() => expect(screen.getByText("agent/ops/runtime")).toBeTruthy());
    expect(screen.getByText("1 total")).toBeTruthy();
    expect(screen.getAllByText("1 identity-bound").length).toBeGreaterThan(0);
    expect(screen.getByText("0 shared")).toBeTruthy();
    expect(screen.getByText("0 secret")).toBeTruthy();
    expect(screen.getByText("allow: ops")).toBeTruthy();
    expect(screen.getByText("deny: blocked")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Agent config key"), { target: { value: "agent/planner/runtime" } });
    fireEvent.change(screen.getByLabelText("Agent config value"), { target: { value: "mode=plan" } });
    fireEvent.change(screen.getByLabelText("Agent config rating"), { target: { value: "restricted" } });
    fireEvent.change(screen.getByLabelText("Allowed agents"), { target: { value: "planner,ops" } });
    fireEvent.change(screen.getByLabelText("Denied agents"), { target: { value: "blocked" } });
    fireEvent.click(screen.getByRole("button", { name: "Save agent config" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/configcenter/set", {
        key: "agent/planner/runtime",
        value: "mode=plan",
        rating: "restricted",
        description: "",
        allowed_agents: ["planner", "ops"],
        excluded_agents: ["blocked"],
      }),
    );
  });

  it("renders an env-pinned field read-only (no input, no save)", async () => {
    mockFetch(valuesPayload({ AGEZT_MODEL: { env_pinned: true, value: "gpt-4o", set: true } }));
    const { container } = render(withUI(<ConfigCenter />));
    await waitFor(() => expect(screen.getByText("env")).toBeTruthy());
    expect(screen.queryByDisplayValue("gpt-4o")).toBeNull();
    expect(container.textContent).toContain("gpt-4o");
  });

  it("clears a secret with an explicit value-empty write", async () => {
    mockFetch();
    postJSON.mockResolvedValueOnce({ env: "AGEZT_TELEGRAM_TOKEN", saved: true, applied: "restart" });
    render(withUI(<ConfigCenter />));
    await waitFor(() => expect(sectionHeading("Telegram")).toBeTruthy());

    fireEvent.click(screen.getByTitle("Clear (remove from vault)"));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TELEGRAM_TOKEN", value: "" }),
    );
  });
});
