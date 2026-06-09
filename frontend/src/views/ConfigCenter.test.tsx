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

import { ConfigCenter } from "@/views/ConfigCenter";
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
      fields: [{ env: "AGEZT_MODEL", label: "Model", type: "text", secret: false, required: false, apply: "live" }],
    },
    {
      id: "telegram",
      name: "Telegram",
      fields: [
        { env: "AGEZT_TELEGRAM_TOKEN", label: "Bot token", type: "password", secret: true, required: false, apply: "restart" },
        { env: "AGEZT_TELEGRAM_CHAT_ID", label: "Allowed chat IDs", type: "csv", secret: false, required: false, apply: "restart" },
      ],
    },
  ],
};

function valuesPayload(over: Record<string, any> = {}) {
  return {
    fields: [
      { env: "AGEZT_MODEL", secret: false, env_pinned: false, set: true, value: "deepseek-chat", ...over.AGEZT_MODEL },
      { env: "AGEZT_TELEGRAM_TOKEN", secret: true, env_pinned: false, set: true, ...over.AGEZT_TELEGRAM_TOKEN },
      { env: "AGEZT_TELEGRAM_CHAT_ID", secret: false, env_pinned: false, set: false, value: "", ...over.AGEZT_TELEGRAM_CHAT_ID },
    ],
  };
}

function mockFetch(values = valuesPayload()) {
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/config/schema") return Promise.resolve(SCHEMA);
    if (path === "/api/config/values") return Promise.resolve(values);
    return Promise.reject(new Error("unexpected " + path));
  });
}

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
});

describe("ConfigCenter view", () => {
  it("renders sections and a field input from the schema", async () => {
    mockFetch();
    render(withUI(<ConfigCenter />));
    await waitFor(() => expect(screen.getByText("Provider & Model")).toBeTruthy());
    expect(screen.getByText("Telegram")).toBeTruthy();
    // Non-secret value is pre-filled.
    expect((screen.getByDisplayValue("deepseek-chat") as HTMLInputElement).value).toBe("deepseek-chat");
  });

  it("never shows a secret value — only presence", async () => {
    mockFetch();
    const { container } = render(withUI(<ConfigCenter />));
    await waitFor(() => expect(screen.getByText("Bot token")).toBeTruthy());
    // The secret input is a password field, empty, with a "set" hint.
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
    // Save via Enter.
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_MODEL", value: "deepseek-reasoner" }),
    );
    await waitFor(() => expect(screen.getByText("Model applied live")).toBeTruthy());
  });

  it("renders an env-pinned field read-only (no input, no save)", async () => {
    mockFetch(valuesPayload({ AGEZT_MODEL: { env_pinned: true, value: "gpt-4o", set: true } }));
    const { container } = render(withUI(<ConfigCenter />));
    await waitFor(() => expect(screen.getByText("env")).toBeTruthy());
    // The pinned model has no editable text input bearing its value.
    expect(screen.queryByDisplayValue("gpt-4o")).toBeNull();
    expect(container.textContent).toContain("gpt-4o");
  });

  it("clears a secret with an explicit value-empty write", async () => {
    mockFetch();
    postJSON.mockResolvedValueOnce({ env: "AGEZT_TELEGRAM_TOKEN", saved: true, applied: "restart" });
    render(withUI(<ConfigCenter />));
    await waitFor(() => expect(screen.getByText("Bot token")).toBeTruthy());

    fireEvent.click(screen.getByTitle("Clear (remove from vault)"));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TELEGRAM_TOKEN", value: "" }),
    );
  });
});
