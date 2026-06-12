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

import { Setup, rankProviders, anyCredentialed, providerKeyEnv, type SetupProvider } from "@/views/Setup";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  postJSON.mockResolvedValue({});
  postAction.mockResolvedValue({});
});

describe("pure helpers", () => {
  it("providerKeyEnv prefers a *_API_KEY / *_KEY / *_TOKEN name, null when keyless", () => {
    expect(providerKeyEnv({ id: "x", env: ["X_BASE", "X_API_KEY"] })).toBe("X_API_KEY");
    expect(providerKeyEnv({ id: "x", env: ["X_TOKEN"] })).toBe("X_TOKEN");
    expect(providerKeyEnv({ id: "x", env: [] })).toBeNull();
    expect(providerKeyEnv({ id: "ollama-local" })).toBeNull();
  });

  it("anyCredentialed reflects the auto-open signal", () => {
    expect(anyCredentialed({ providers: [{ id: "a" }, { id: "b", credentialed: true }] })).toBe(true);
    expect(anyCredentialed({ providers: [{ id: "a" }] })).toBe(false);
    expect(anyCredentialed(null)).toBe(false);
  });

  it("rankProviders surfaces credentialed, then keyed, then local; query filters", () => {
    const ps: SetupProvider[] = [
      { id: "local", env: [] },
      { id: "openai", env: ["OPENAI_API_KEY"] },
      { id: "minimax", env: ["MINIMAX_API_KEY"], credentialed: true },
    ];
    expect(rankProviders(ps, "").map((p) => p.id)).toEqual(["minimax", "openai", "local"]);
    expect(rankProviders(ps, "open").map((p) => p.id)).toEqual(["openai"]);
  });
});

describe("Setup wizard", () => {
  const catalog = {
    providers: [
      { id: "minimax", env: ["MINIMAX_API_KEY"], credentialed: false, model_count: 2, models: [{ id: "MiniMax-M2" }] },
    ],
    provider_count: 1,
  };

  it("walks catalog → key → model and finishes via onDone", async () => {
    getJSON.mockResolvedValue(catalog);
    const onDone = vi.fn();
    render(withUI(<Setup onDone={onDone} />));

    // Catalog non-empty → jumps to the provider step.
    await waitFor(() => expect(screen.getByText("Choose a provider and add its key")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "Pick minimax" }));
    fireEvent.change(screen.getByLabelText("API key"), { target: { value: "sk-secret-123" } });
    fireEvent.click(screen.getByRole("button", { name: "Use minimax" }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/provider/keys/add", {
        env: "MINIMAX_API_KEY",
        label: "default",
        value: "sk-secret-123",
        active: "true",
      }),
    );
    expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_PROVIDER", value: "minimax" });
    expect(postAction).toHaveBeenCalledWith("/api/provider/reload", {});

    // Model step → pick the one model → sets AGEZT_MODEL → password step.
    await waitFor(() => expect(screen.getByRole("button", { name: "Use model MiniMax-M2" })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Use model MiniMax-M2" }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_MODEL", value: "MiniMax-M2" }),
    );

    // Console password step (M933): set one → saved as the vault secret.
    await waitFor(() => expect(screen.getByText("Console password (optional)")).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Console password"), { target: { value: "hunter2" } });
    fireEvent.change(screen.getByLabelText("Repeat console password"), { target: { value: "hunter2" } });
    fireEvent.click(screen.getByRole("button", { name: "Set console password" }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_WEB_PASSWORD", value: "hunter2" }),
    );

    await waitFor(() => expect(screen.getByText("You're ready")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Start chatting" }));
    expect(onDone).toHaveBeenCalled();
  });

  it("password step is skippable", async () => {
    getJSON.mockResolvedValue(catalog);
    render(withUI(<Setup />));
    await waitFor(() => expect(screen.getByText("Choose a provider and add its key")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Pick minimax" }));
    fireEvent.change(screen.getByLabelText("API key"), { target: { value: "sk-x" } });
    fireEvent.click(screen.getByRole("button", { name: "Use minimax" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Use model MiniMax-M2" })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Use model MiniMax-M2" }));
    await waitFor(() => expect(screen.getByText("Console password (optional)")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Skip password" }));
    await waitFor(() => expect(screen.getByText("You're ready")).toBeTruthy());
    // Skipping never writes the password secret.
    expect(postJSON).not.toHaveBeenCalledWith("/api/config/set", expect.objectContaining({ name: "AGEZT_WEB_PASSWORD" }));
  });

  it("offers Sync when the catalog is empty", async () => {
    getJSON.mockResolvedValueOnce({ providers: [] }).mockResolvedValue(catalog);
    render(withUI(<Setup />));
    await waitFor(() => expect(screen.getByText("Model catalog")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Sync catalog" }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/catalog/sync", {}));
  });

  it("overlay mode offers Skip", async () => {
    getJSON.mockResolvedValue(catalog);
    const onSkip = vi.fn();
    render(withUI(<Setup overlay onSkip={onSkip} />));
    await waitFor(() => expect(screen.getByRole("button", { name: "Skip setup" })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Skip setup" }));
    expect(onSkip).toHaveBeenCalled();
  });
});
