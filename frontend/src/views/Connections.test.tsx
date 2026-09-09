// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));

import { Connections, ConnectivityStrip } from "@/views/Connections";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset().mockResolvedValue({});
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/catalog") {
      return Promise.resolve({
        providers: [
          { id: "deepseek", name: "DeepSeek", credentialed: true, env: ["DEEPSEEK_API_KEY"], family: "openai-compatible", api: "https://api.deepseek.com", model_count: 1, models: [{ id: "deepseek-chat" }] },
          { id: "openai", name: "OpenAI", credentialed: false, env: ["OPENAI_API_KEY"], family: "openai", model_count: 2, models: [{ id: "gpt-4o" }, { id: "gpt-4o-mini" }] },
          { id: "ollama", name: "Ollama", credentialed: false, env: [], family: "ollama", api: "http://localhost:11434/v1", model_count: 1, models: [{ id: "llama3.2" }] },
        ],
      });
    }
    if (path === "/api/channels") return Promise.resolve({ channels: [{ kind: "telegram", display: "Telegram", live: true, configured: true }, { kind: "irc", display: "IRC", live: false, configured: true }] });
    if (path === "/api/mcp") return Promise.resolve({ servers: [{ name: "fetch", attached: true, enabled: true }] });
    if (path === "/api/nodes") return Promise.resolve({ nodes: [{ name: "local", local: true, reachable: true }, { name: "nodeB", reachable: true, version: "peer-1" }] });
    return Promise.resolve({});
  });
});
afterEach(cleanup);

describe("Connections · Status tab", () => {
  it("summarizes connected providers, channels, MCP servers, and nodes", async () => {
    render(<Connections />);
    expect(await screen.findByText("Connections")).toBeTruthy();
    expect(await screen.findByText("DeepSeek")).toBeTruthy();
    expect(screen.getByText("Telegram")).toBeTruthy();
    expect(screen.getByText(/restart to start/i)).toBeTruthy();
    expect(screen.getByText("fetch")).toBeTruthy();
    expect(screen.getByText("nodeB")).toBeTruthy();
  });

  it("'Add provider' switches to the Provider Keys tab (no longer navigates to #quickconnect)", async () => {
    // Switching tabs mounts the ProviderKeysTab which uses useUI — wrap.
    render(withUI(<Connections />));
    await screen.findByText("Connections");
    fireEvent.click(screen.getByRole("button", { name: /add provider/i }));
    // The new flow lives inside the Connections view itself — the Provider
    // Keys heading and the "taken from the catalog" preamble are visible.
    await screen.findByText(/env var names are taken from the catalog/i);
    expect(location.hash).not.toBe("#quickconnect");
  });

  it("'Manage channels' still navigates to #channels", async () => {
    render(<Connections />);
    await screen.findByText("Connections");
    fireEvent.click(screen.getByRole("button", { name: /manage channels/i }));
    expect(location.hash).toBe("#channels");
  });
});

describe("Connections · Provider Keys tab (replaces the old Quick Connect gallery)", () => {
  it("sources env var names from the catalog (no per-preset hard-coding)", async () => {
    render(withUI(<Connections />));
    await screen.findByText("Connections");
    fireEvent.click(screen.getByRole("tab", { name: /provider keys/i }));
    await screen.findByText(/env var names are taken from the catalog/i);
    // DeepSeek's env from the catalog (DEEPSEEK_API_KEY) is shown, not a
    // preset-baked alternative.
    expect(screen.getByText("DEEPSEEK_API_KEY")).toBeTruthy();
    expect(screen.getByText("OPENAI_API_KEY")).toBeTruthy();
  });

  it("attaches a key to a catalog-known provider using the catalog's env", async () => {
    render(withUI(<Connections />));
    await screen.findByText("Connections");
    fireEvent.click(screen.getByRole("tab", { name: /provider keys/i }));
    await screen.findByText("DEEPSEEK_API_KEY");

    // Use OpenAI (unkeyed in the mock) so the button label is "Save key";
    // DeepSeek is already keyed so it would render as "Replace key".
    const openaiKey = screen.getByLabelText("OpenAI key");
    fireEvent.change(openaiKey, { target: { value: "sk-test" } });
    fireEvent.click(screen.getByRole("button", { name: /save key/i }));

    await waitFor(() => expect(postJSON).toHaveBeenCalledWith(
      "/api/provider/keys/add",
      expect.objectContaining({
        provider: "openai",
        env: "OPENAI_API_KEY",
        value: "sk-test",
        active: true,
      }),
    ));
  });

  it("can pin a default model and set it as the default brain in one step", async () => {
    render(withUI(<Connections />));
    await screen.findByText("Connections");
    fireEvent.click(screen.getByRole("tab", { name: /provider keys/i }));
    await screen.findByText("OPENAI_API_KEY");

    // OpenAI has 2 models, default "gpt-4o". Switch to gpt-4o-mini and tick
    // the "set as default brain" checkbox.
    const modelSelect = screen.getByLabelText("OpenAI default model") as HTMLSelectElement;
    fireEvent.change(modelSelect, { target: { value: "gpt-4o-mini" } });
    const key = screen.getByLabelText("OpenAI key");
    fireEvent.change(key, { target: { value: "sk-test" } });
    fireEvent.click(screen.getByLabelText("Set OpenAI as default brain"));
    fireEvent.click(screen.getAllByRole("button", { name: /save key/i })[0]);

    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_PROVIDER", value: "openai" }));
    expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_MODEL", value: "gpt-4o-mini" });
    expect(postJSON).toHaveBeenCalledWith("/api/provider/reload", {});
  });

  it("connects a keyless local runtime in one click (env is empty)", async () => {
    render(withUI(<Connections />));
    await screen.findByText("Connections");
    fireEvent.click(screen.getByRole("tab", { name: /provider keys/i }));
    // Wait for the catalog to load (3 providers including ollama).
    await screen.findByText("DEEPSEEK_API_KEY");
    // Ollama is keyless → one-click connect.
    const ollamaRow = screen.getByText("ollama").closest("div.glass") as HTMLElement;
    fireEvent.click(within(ollamaRow).getByRole("button", { name: /connect/i }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith(
      "/api/provider/connect",
      expect.objectContaining({ id: "ollama", env: "" }),
    ));
  });
});

describe("ConnectivityStrip", () => {
  it("summarizes counts and links to the cockpit", async () => {
    location.hash = "";
    render(<ConnectivityStrip />);
    const btn = await screen.findByRole("button", { name: /connections/i });
    expect(btn.textContent).toMatch(/1 provider keyed/i);
    expect(btn.textContent).toMatch(/1 channel live/i);
    expect(btn.textContent).toMatch(/1 MCP attached/i);
    expect(btn.textContent).toMatch(/2 nodes reachable/i);
    fireEvent.click(btn);
    expect(location.hash).toBe("#connections");
  });
});
