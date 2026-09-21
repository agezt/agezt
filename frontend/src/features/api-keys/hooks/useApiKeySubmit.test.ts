// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";
import * as React from "react";
import { useApiKeySubmit, type UseApiKeySubmitOptions } from "./useApiKeySubmit";
import { UIProvider } from "@/components/ui/feedback";
import { postJSON, postAction } from "@/app/api";

vi.mock("@/app/api", () => ({
  postJSON: vi.fn(),
  postAction: vi.fn(),
}));

const mockedPostJSON = vi.mocked(postJSON);
const mockedPostAction = vi.mocked(postAction);

afterEach(() => {
  cleanup();
  mockedPostJSON.mockReset();
  mockedPostAction.mockReset();
});

// api-key hook tests are forced through React.createElement because the OXC
// parser used by Vite 8 chokes on locally-declared React components inside
// `it` callbacks (gives "Expected `>` but found `/`" parse errors). Helpers
// here take an `apiRef` and write to it from a top-level render.

function renderWithHook(opts: UseApiKeySubmitOptions, apiRef: { current: ReturnType<typeof useApiKeySubmit> | undefined }) {
  function Harness() {
    apiRef.current = useApiKeySubmit(opts);
    return null;
  }
  return render(React.createElement(UIProvider, null, React.createElement(Harness)));
}

describe("useApiKeySubmit", () => {
  beforeEach(() => {
    mockedPostJSON.mockResolvedValue({ ok: true } as never);
    mockedPostAction.mockResolvedValue({ ok: true } as never);
  });

  it("returns a usable API object with add/activate/remove/busy", () => {
    const ref: { current: ReturnType<typeof useApiKeySubmit> | undefined } = { current: undefined };
    renderWithHook({}, ref);
    expect(ref.current).toBeDefined();
    expect(ref.current!.busy).toBe(false);
    expect(typeof ref.current!.add).toBe("function");
    expect(typeof ref.current!.activate).toBe("function");
    expect(typeof ref.current!.remove).toBe("function");
  });

  it("add posts the keys/add endpoint and refreshes", async () => {
    const onSuccess = vi.fn();
    const ref: { current: ReturnType<typeof useApiKeySubmit> | undefined } = { current: undefined };
    renderWithHook({ onSuccess }, ref);
    const ok = await ref.current!.add({ env: "OPENAI_API_KEY", label: "work", value: "sk-1234", active: true });
    expect(ok).toBe(true);
    expect(mockedPostJSON).toHaveBeenCalledWith("/api/provider/keys/add", {
      provider: undefined,
      env: "OPENAI_API_KEY",
      label: "work",
      value: "sk-1234",
      active: true,
    });
    await waitFor(() => expect(onSuccess).toHaveBeenCalledOnce());
  });

  it("activate posts the activate endpoint", async () => {
    const onSuccess = vi.fn();
    const ref: { current: ReturnType<typeof useApiKeySubmit> | undefined } = { current: undefined };
    renderWithHook({ onSuccess }, ref);
    const ok = await ref.current!.activate("OPENAI_API_KEY", "personal");
    expect(ok).toBe(true);
    expect(mockedPostAction).toHaveBeenCalledWith("/api/provider/keys/activate", { env: "OPENAI_API_KEY", label: "personal" });
    await waitFor(() => expect(onSuccess).toHaveBeenCalledOnce());
  });

  it("remove posts the remove endpoint", async () => {
    const onSuccess = vi.fn();
    const ref: { current: ReturnType<typeof useApiKeySubmit> | undefined } = { current: undefined };
    renderWithHook({ onSuccess }, ref);
    const ok = await ref.current!.remove("OPENAI_API_KEY", "old");
    expect(ok).toBe(true);
    expect(mockedPostAction).toHaveBeenCalledWith("/api/provider/keys/remove", { env: "OPENAI_API_KEY", label: "old" });
    await waitFor(() => expect(onSuccess).toHaveBeenCalledOnce());
  });

  it("returns false and toasts on failure", async () => {
    mockedPostJSON.mockRejectedValueOnce(new Error("daemon down"));
    const ref: { current: ReturnType<typeof useApiKeySubmit> | undefined } = { current: undefined };
    renderWithHook({}, ref);
    const ok = await ref.current!.add({ env: "OPENAI_API_KEY", label: "x", value: "y" });
    expect(ok).toBe(false);
  });

  it("uses a custom successMessage when provided", async () => {
    const successMessage = vi.fn().mockReturnValue("custom ok");
    const ref: { current: ReturnType<typeof useApiKeySubmit> | undefined } = { current: undefined };
    renderWithHook({ successMessage }, ref);
    await ref.current!.add({ env: "OPENAI_API_KEY", label: "x", value: "y" });
    expect(successMessage).toHaveBeenCalledWith(expect.objectContaining({ env: "OPENAI_API_KEY", label: "x" }));
  });
});
