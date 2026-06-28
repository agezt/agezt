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

import { QuickConnect } from "@/views/QuickConnect";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

beforeEach(() => {
  getJSON.mockReset().mockResolvedValue({ providers: [] });
  postJSON.mockReset().mockResolvedValue({});
});
afterEach(cleanup);

describe("QuickConnect", () => {
  it("renders the branded gallery with coding-plan + popular cards", async () => {
    render(withUI(<QuickConnect />));
    await screen.findByText("Quick Connect");
    expect(screen.getAllByText("Z.ai API").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Z.ai Coding Plan").length).toBeGreaterThan(0);
    expect(screen.getAllByText("DeepSeek").length).toBeGreaterThan(0);
    expect(screen.getByText("OpenRouter")).toBeTruthy();
    expect(screen.getByText("Custom provider")).toBeTruthy();
  });

  it("connect registers the provider then stores the key", async () => {
    render(withUI(<QuickConnect />));
    await screen.findByText("Quick Connect");

    // DeepSeek's card opens a modal; the key is entered there.
    const deepseek = screen.getAllByText("DeepSeek")[0];
    const deepseekCard = deepseek.closest("div.glass")!;
    const openBtn = Array.from(deepseekCard.querySelectorAll("button")).find((b) => /^connect$/i.test(b.textContent || ""))!;
    fireEvent.click(openBtn);
    fireEvent.change(screen.getByLabelText("DeepSeek API key"), { target: { value: "sk-test" } });
    const connectBtn = screen.getByRole("button", { name: "Connect DeepSeek" });
    fireEvent.click(connectBtn);

    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/provider/connect", expect.objectContaining({
      id: "deepseek",
      env: "DEEPSEEK_API_KEY",
      npm: "@ai-sdk/openai-compatible",
      api: "https://api.deepseek.com",
      model: "deepseek-chat",
    })));
    expect(postJSON).toHaveBeenCalledWith("/api/provider/keys/add", expect.objectContaining({
      provider: "deepseek",
      env: "DEEPSEEK_API_KEY",
      value: "sk-test",
      active: true,
    }));
  });

  it("optionally pins the provider as default brain", async () => {
    render(withUI(<QuickConnect />));
    await screen.findByText("Quick Connect");

    const deepseek = screen.getAllByText("DeepSeek")[0];
    const deepseekCard = deepseek.closest("div.glass")!;
    const openBtn = Array.from(deepseekCard.querySelectorAll("button")).find((b) => /^connect$/i.test(b.textContent || ""))!;
    fireEvent.click(openBtn);
    fireEvent.change(screen.getByLabelText("DeepSeek API key"), { target: { value: "sk-test" } });
    // Tick "Set as default brain" within the DeepSeek card.
    const checkbox = screen.getByLabelText("Set as default brain") as HTMLInputElement;
    fireEvent.click(checkbox);
    const connectBtn = screen.getByRole("button", { name: "Connect DeepSeek" });
    fireEvent.click(connectBtn);

    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_PROVIDER", value: "deepseek" }));
    expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_MODEL", value: "deepseek-chat" });
    expect(postJSON).toHaveBeenCalledWith("/api/provider/reload", {});
  });

  it("connects a keyless local runtime with no key (env empty)", async () => {
    render(withUI(<QuickConnect />));
    await screen.findByText("Quick Connect");
    const ollama = screen.getByText("Ollama");
    const card = ollama.closest("div.glass")!;
    const connectBtn = Array.from(card.querySelectorAll("button")).find((b) => /connect/i.test(b.textContent || ""))!;
    fireEvent.click(connectBtn);
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/provider/connect", expect.objectContaining({
        id: "ollama",
        env: "",
        api: "http://localhost:11434/v1",
      })),
    );
    // No keyring write for a keyless runtime.
    expect(postJSON).not.toHaveBeenCalledWith("/api/provider/keys/add", expect.anything());
  });

  it("probes endpoint reachability via Check", async () => {
    postJSON.mockResolvedValue({ ok: true, reachable: true, authorized: true, models: 7 });
    render(withUI(<QuickConnect />));
    await screen.findByText("Quick Connect");
    const ollama = screen.getByText("Ollama");
    const card = ollama.closest("div.glass")!;
    const checkBtn = Array.from(card.querySelectorAll("button")).find((b) => /check/i.test(b.textContent || ""))!;
    fireEvent.click(checkBtn);
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/provider/probe", expect.objectContaining({ url: "http://localhost:11434/v1" })));
    expect(await screen.findByText(/reachable \(7 models\)/i)).toBeTruthy();
  });

  it("refuses to connect without a key", async () => {
    render(withUI(<QuickConnect />));
    await screen.findByText("Quick Connect");
    const groq = screen.getByText("Groq");
    const card = groq.closest("div.glass")!;
    const openBtn = Array.from(card.querySelectorAll("button")).find((b) => /^connect$/i.test(b.textContent || ""))!;
    fireEvent.click(openBtn);
    const connectBtn = screen.getByRole("button", { name: "Connect Groq" });
    fireEvent.click(connectBtn);
    // No connect call fired (key empty → toast, early return).
    await waitFor(() => expect(postJSON).not.toHaveBeenCalledWith("/api/provider/connect", expect.anything()));
  });

  it("connects a custom Anthropic-compatible provider from compatibility chips", async () => {
    render(withUI(<QuickConnect />));
    await screen.findByText("Quick Connect");

    const custom = screen.getByText("Custom provider");
    const card = custom.closest("div.glass")!;
    fireEvent.click(within(card as HTMLElement).getByRole("button", { name: /Configure/i }));
    fireEvent.change(screen.getByLabelText("Provider name"), { target: { value: "Claude Proxy" } });
    fireEvent.click(within(screen.getByRole("group", { name: "Compatibility" })).getByRole("button", { name: /Anthropic-compatible/i }));
    fireEvent.change(screen.getByLabelText("Base URL"), { target: { value: "https://claude-proxy.test" } });
    fireEvent.change(screen.getByLabelText("Model"), { target: { value: "claude-test" } });
    fireEvent.change(screen.getByLabelText("Custom provider API key"), { target: { value: "sk-custom" } });
    fireEvent.click(within(screen.getByRole("dialog", { name: "Custom provider" })).getByRole("button", { name: "Connect" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/provider/connect",
        expect.objectContaining({
          id: "claude-proxy",
          name: "Claude Proxy",
          npm: "@ai-sdk/anthropic",
          api: "https://claude-proxy.test",
          model: "claude-test",
        }),
      ),
    );
  });
});
