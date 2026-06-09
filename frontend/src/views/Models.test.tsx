// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { Models } from "@/views/Models";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

const CATALOG = {
  provider_count: 2,
  api_synced_at: "2026-06-01T12:00:00Z",
  api_source_url: "https://models.dev/api.json",
  providers: [
    {
      id: "deepseek",
      name: "DeepSeek",
      credentialed: true,
      model_count: 1,
      models: [{ id: "deepseek-chat", context: 64000, tool_call: true, cost_input_usd_per_mtok: 0.27 }],
    },
    {
      id: "openai",
      name: "OpenAI",
      credentialed: false,
      model_count: 1,
      models: [{ id: "gpt-4o", context: 128000, reasoning: true }],
    },
  ],
};

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  getJSON.mockResolvedValue(CATALOG);
});

describe("Models view", () => {
  it("renders providers with keyed/no-key badges and last-synced", async () => {
    render(withUI(<Models />));
    await waitFor(() => expect(screen.getByText("DeepSeek")).toBeTruthy());
    expect(screen.getByText("OpenAI")).toBeTruthy();
    expect(screen.getByText("keyed")).toBeTruthy();
    expect(screen.getByText("no key")).toBeTruthy();
    expect(screen.getByText(/Last synced/)).toBeTruthy();
    expect(screen.getByText("2 providers · 2 models")).toBeTruthy();
  });

  it("expands a provider to show its models", async () => {
    render(withUI(<Models />));
    await waitFor(() => expect(screen.getByText("DeepSeek")).toBeTruthy());
    fireEvent.click(screen.getByText("DeepSeek"));
    expect(screen.getByText("deepseek-chat")).toBeTruthy();
  });

  it("syncs from models.dev and toasts the result", async () => {
    postJSON.mockResolvedValueOnce({ provider_count: 21, model_count: 210 });
    render(withUI(<Models />));
    await waitFor(() => expect(screen.getByText("DeepSeek")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: /Sync models/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/catalog/sync", {}));
    await waitFor(() => expect(screen.getByText("Synced 21 providers · 210 models")).toBeTruthy());
  });

  it("filters by the search query", async () => {
    render(withUI(<Models />));
    await waitFor(() => expect(screen.getByText("DeepSeek")).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Search models"), { target: { value: "gpt" } });
    await waitFor(() => expect(screen.queryByText("DeepSeek")).toBeNull());
    expect(screen.getByText("OpenAI")).toBeTruthy();
    expect(screen.getByText("gpt-4o")).toBeTruthy();
  });

  it("shows a never-synced hint when there is no sync time", async () => {
    getJSON.mockResolvedValue({ ...CATALOG, api_synced_at: "0001-01-01T00:00:00Z" });
    render(withUI(<Models />));
    await waitFor(() => expect(screen.getByText(/Never synced/)).toBeTruthy());
  });
});

describe("Models key management", () => {
  const KEYED = {
    ...CATALOG,
    providers: [
      {
        id: "openai",
        name: "OpenAI",
        credentialed: true,
        env: ["OPENAI_API_KEY"],
        model_count: 1,
        models: [{ id: "gpt-4o", context: 128000 }],
      },
    ],
  };

  function mockWithKeys(keys: any[]) {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/catalog") return Promise.resolve(KEYED);
      if (path === "/api/provider/keys") return Promise.resolve({ env: "OPENAI_API_KEY", keys });
      return Promise.reject(new Error("unexpected " + path));
    });
  }

  it("lists a provider's keys (label + fingerprint, never the value) on expand", async () => {
    mockWithKeys([
      { label: "work", active: true, last4: "…1111" },
      { label: "personal", active: false, last4: "…2222" },
    ]);
    render(withUI(<Models />));
    await waitFor(() => expect(screen.getByText("OpenAI")).toBeTruthy());
    fireEvent.click(screen.getByText("OpenAI"));

    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/provider/keys", { env: "OPENAI_API_KEY" }));
    expect(screen.getByText("work")).toBeTruthy();
    expect(screen.getByText("…1111")).toBeTruthy();
    expect(screen.getByText("personal")).toBeTruthy();
    // Active key shows "active"; the inactive one offers "activate".
    expect(screen.getByText("active")).toBeTruthy();
    expect(screen.getByText("activate")).toBeTruthy();
  });

  it("adds a new key via the add form", async () => {
    mockWithKeys([{ label: "work", active: true, last4: "…1111" }]);
    postJSON.mockResolvedValueOnce({ added: true, active_changed: false });
    render(withUI(<Models />));
    await waitFor(() => expect(screen.getByText("OpenAI")).toBeTruthy());
    fireEvent.click(screen.getByText("OpenAI"));
    await waitFor(() => expect(screen.getByLabelText("New key label")).toBeTruthy());

    fireEvent.change(screen.getByLabelText("New key label"), { target: { value: "personal" } });
    fireEvent.change(screen.getByLabelText("New key value"), { target: { value: "sk-secret-xyz" } });
    fireEvent.click(screen.getByRole("button", { name: /Add/ }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/provider/keys/add", {
        env: "OPENAI_API_KEY",
        label: "personal",
        value: "sk-secret-xyz",
        active: false,
      }),
    );
  });

  it("activates an inactive key", async () => {
    mockWithKeys([
      { label: "work", active: true, last4: "…1111" },
      { label: "personal", active: false, last4: "…2222" },
    ]);
    postAction.mockResolvedValueOnce({ active: true });
    render(withUI(<Models />));
    await waitFor(() => expect(screen.getByText("OpenAI")).toBeTruthy());
    fireEvent.click(screen.getByText("OpenAI"));
    await waitFor(() => expect(screen.getByText("activate")).toBeTruthy());

    fireEvent.click(screen.getByText("activate"));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/provider/keys/activate", { env: "OPENAI_API_KEY", label: "personal" }),
    );
  });
});
