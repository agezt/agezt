// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
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
    expect(screen.getAllByText("Z.ai GLM").length).toBeGreaterThan(0); // two cards (OpenAI + Anthropic)
    expect(screen.getAllByText("DeepSeek").length).toBeGreaterThan(0);
    expect(screen.getByText("OpenRouter")).toBeTruthy();
    expect(screen.getByText("Custom provider")).toBeTruthy();
  });

  it("connect registers the provider then stores the key", async () => {
    render(withUI(<QuickConnect />));
    await screen.findByText("Quick Connect");

    // DeepSeek's (OpenAI-compat) key field, then its Connect button.
    fireEvent.change(screen.getByLabelText("DeepSeek API key"), { target: { value: "sk-test" } });
    const keyInput = screen.getByLabelText("DeepSeek API key");
    const card = keyInput.closest("div.glass") ?? keyInput.parentElement!.parentElement!;
    const connectBtn = Array.from(card.querySelectorAll("button")).find((b) => /connect/i.test(b.textContent || ""))!;
    fireEvent.click(connectBtn);

    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/provider/connect", expect.objectContaining({
      id: "deepseek",
      env: "DEEPSEEK_API_KEY",
      npm: "@ai-sdk/openai-compatible",
      api: "https://api.deepseek.com",
      model: "deepseek-chat",
    })));
    expect(postJSON).toHaveBeenCalledWith("/api/provider/keys/add", expect.objectContaining({
      env: "DEEPSEEK_API_KEY",
      value: "sk-test",
      active: "true",
    }));
  });

  it("optionally pins the provider as default brain", async () => {
    render(withUI(<QuickConnect />));
    await screen.findByText("Quick Connect");

    fireEvent.change(screen.getByLabelText("DeepSeek API key"), { target: { value: "sk-test" } });
    const keyInput = screen.getByLabelText("DeepSeek API key");
    const card = keyInput.closest("div.glass") ?? keyInput.parentElement!.parentElement!;
    // Tick "Set as default brain" within the DeepSeek card.
    const checkbox = card.querySelector('input[type="checkbox"]') as HTMLInputElement;
    fireEvent.click(checkbox);
    const connectBtn = Array.from(card.querySelectorAll("button")).find((b) => /connect/i.test(b.textContent || ""))!;
    fireEvent.click(connectBtn);

    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_PROVIDER", value: "deepseek" }));
    expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_MODEL", value: "deepseek-chat" });
    expect(postJSON).toHaveBeenCalledWith("/api/provider/reload", {});
  });

  it("refuses to connect without a key", async () => {
    render(withUI(<QuickConnect />));
    await screen.findByText("Quick Connect");
    const keyInput = screen.getByLabelText("Groq Fast inference key");
    const card = keyInput.closest("div.glass") ?? keyInput.parentElement!.parentElement!;
    const connectBtn = Array.from(card.querySelectorAll("button")).find((b) => /connect/i.test(b.textContent || ""))!;
    fireEvent.click(connectBtn);
    // No connect call fired (key empty → toast, early return).
    await waitFor(() => expect(postJSON).not.toHaveBeenCalledWith("/api/provider/connect", expect.anything()));
  });
});
