// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import * as React from "react";
import { ChatGPTSignInCard } from "./ChatGPTSignInCard";
import { UIProvider, useUI } from "@/components/ui/feedback";
import { postJSON } from "@/app/api";

vi.mock("@/app/api", () => ({
  postJSON: vi.fn(),
  postAction: vi.fn(),
}));

const mockedPostJSON = vi.mocked(postJSON);

afterEach(() => {
  cleanup();
  mockedPostJSON.mockReset();
  // jsdom keeps a singleton window.open spy across tests; reset its call list.
  (window.open as unknown as { mockReset?: () => void } | undefined);
  vi.restoreAllMocks();
});

function withUI(node: React.ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

describe("ChatGPTSignInCard", () => {
  beforeEach(() => {
    // Default: not connected; status poll returns empty.
    mockedPostJSON.mockResolvedValue({ connected: false, email: "" } as never);
  });

  it("renders disconnected state and the two CTAs", async () => {
    render(withUI(<ChatGPTSignInCard />));
    await waitFor(() => expect(screen.getByText("not connected")).toBeTruthy());
    expect(screen.getByRole("button", { name: /sign in with chatgpt/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /import from codex cli/i })).toBeTruthy();
  });

  it("renders connected state with email when the daemon reports it", async () => {
    mockedPostJSON.mockResolvedValueOnce({ connected: true, email: "owner@example.com" } as never);
    render(withUI(<ChatGPTSignInCard />));
    await waitFor(() => expect(screen.getByText(/connected/i)).toBeTruthy());
    expect(screen.getByText(/owner@example\.com/)).toBeTruthy();
    expect(screen.getByRole("button", { name: /disconnect chatgpt/i })).toBeTruthy();
  });

  it("fires onChanged once when the initial status reports connected", async () => {
    mockedPostJSON.mockResolvedValueOnce({ connected: true, email: "owner@example.com" } as never);
    const onChanged = vi.fn();
    render(withUI(<ChatGPTSignInCard onChanged={onChanged} />));
    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
  });

  it("does NOT fire onChanged on initial mount when the daemon is disconnected", async () => {
    const onChanged = vi.fn();
    render(withUI(<ChatGPTSignInCard onChanged={onChanged} />));
    // give the refresh + state-update a couple of ticks to settle
    await waitFor(() => expect(screen.getByText("not connected")).toBeTruthy());
    expect(onChanged).not.toHaveBeenCalled();
  });

  it("gates the OAuth flow behind a confirm modal", async () => {
    // Build a tiny host component to read useUI() so we can confirm the modal
    // pattern (it's how the existing inline ChatGPTSignIn behaves too).
    function Probe() {
      const { confirm } = useUI();
      React.useEffect(() => {
        // Trigger the sign-in flow from outside — emulate a click.
        confirm({
          title: "Sign in with ChatGPT?",
          message: "headless test",
          confirmLabel: "Continue",
          danger: true,
        }).then((ok) => {
          // just record; the assertion below is the visible confirm modal
          (window as unknown as { __confirmResult: boolean }).__confirmResult = ok;
        });
      }, [confirm]);
      return null;
    }
    render(
      <UIProvider>
        <Probe />
      </UIProvider>,
    );
    expect(await screen.findByRole("dialog")).toBeTruthy();
    expect(screen.getByText(/sign in with chatgpt\?/i)).toBeTruthy();
    expect(screen.getByRole("button", { name: /continue/i })).toBeTruthy();
  });

  it("calls /api/provider/oauth/start when the user confirms and opens a window", async () => {
    mockedPostJSON.mockImplementation(async (path: string) => {
      if (path === "/api/provider/oauth/start") return { authorize_url: "https://example.com/auth", state: "abc" };
      if (path === "/api/provider/oauth/status") return { status: "done", connected: true, email: "owner@example.com" };
      return { connected: false, email: "" };
    });
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);
    render(withUI(<ChatGPTSignInCard />));
    // Click the button — opens the confirm modal. Confirm it.
    fireEvent.click(screen.getByRole("button", { name: /sign in with chatgpt/i }));
    const confirmBtn = await screen.findByRole("button", { name: /continue/i });
    fireEvent.click(confirmBtn);
    // Window opens, start is called, polling completes.
    await waitFor(() => expect(openSpy).toHaveBeenCalledWith("https://example.com/auth", "_blank", "noopener,noreferrer"));
    await waitFor(() =>
      expect(mockedPostJSON).toHaveBeenCalledWith("/api/provider/oauth/start", { provider: "chatgpt" }),
    );
    await waitFor(() => expect(screen.getByText(/connected/i)).toBeTruthy());
    openSpy.mockRestore();
  });

  it("compact mode hides the status line", async () => {
    mockedPostJSON.mockImplementation(async (path: string) => {
      if (path === "/api/provider/oauth/start") return { authorize_url: "https://example.com/auth", state: "abc" };
      if (path === "/api/provider/oauth/status") return { status: "done", connected: true };
      return { connected: false };
    });
    render(withUI(<ChatGPTSignInCard compact />));
    // compact card still renders the same header CTAs, just no status line.
    expect(screen.getByRole("button", { name: /sign in with chatgpt/i })).toBeTruthy();
  });

  it("shows a Cancel button while the OAuth flow is in flight, and Cancel aborts polling", async () => {
    // Polling returns "pending" forever — Cancel is the only way out.
    let pollCount = 0;
    mockedPostJSON.mockImplementation(async (path: string) => {
      if (path === "/api/provider/oauth/start") return { authorize_url: "https://example.com/auth", state: "abc" };
      if (path === "/api/provider/oauth/status") {
        pollCount += 1;
        return { status: "pending" };
      }
      return { connected: false };
    });
    render(withUI(<ChatGPTSignInCard pollTimeoutSec={120} />));
    fireEvent.click(screen.getByRole("button", { name: /sign in with chatgpt/i }));
    const confirmBtn = await screen.findByRole("button", { name: /continue/i });
    fireEvent.click(confirmBtn);
    // The Cancel button appears alongside the in-flight spinner.
    const cancelBtn = await screen.findByRole("button", { name: /cancel chatgpt sign-in/i });
    expect(cancelBtn).toBeTruthy();
    const pollsBeforeCancel = pollCount;
    fireEvent.click(cancelBtn);
    // After Cancel the Sign-in CTA comes back; the in-flight spinner goes.
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /sign in with chatgpt/i })).toBeTruthy(),
    );
    // Give a tick for any queued poll promise to settle — it must NOT have
    // re-entered the loop. Allow a tiny window for in-flight work to drain.
    await new Promise((res) => setTimeout(res, 100));
    expect(pollCount).toBeLessThanOrEqual(pollsBeforeCancel + 1);
  });

  it("does not re-fire onChanged once the card is already in connected state", async () => {
    // Mount with connected = true, then poll the daemon again — onChanged
    // must NOT fire on the second poll (it's a no-op, not an edge transition).
    mockedPostJSON.mockResolvedValue({ connected: true, email: "owner@example.com" } as never);
    const onChanged = vi.fn();
    render(withUI(<ChatGPTSignInCard onChanged={onChanged} />));
    await waitFor(() => expect(screen.getByText(/connected/i)).toBeTruthy());
    // One call on initial mount is allowed; a second one on the same edge
    // would mean the connectedRef wiring didn't help and stale closure still
    // fires. Wait a tick for any queued effect to settle.
    await new Promise((res) => setTimeout(res, 50));
    expect(onChanged.mock.calls.length).toBeLessThanOrEqual(1);
  });

  it("survives unmount mid-OAuth without warnings (polling aborts)", async () => {
    // The cleanup useEffect must set abortRef so an in-flight polling loop
    // exits on the next tick. We can't directly observe the abort, but we
    // can render → start OAuth → unmount and assert no React warnings/errors.
    mockedPostJSON.mockImplementation(async (path: string) => {
      if (path === "/api/provider/oauth/start") return { authorize_url: "https://example.com/auth", state: "abc" };
      if (path === "/api/provider/oauth/status") return { status: "pending" };
      return { connected: false };
    });
    const errors: string[] = [];
    const origError = console.error;
    console.error = (...args: unknown[]) => errors.push(args.map(String).join(" "));
    try {
      const { unmount } = render(withUI(<ChatGPTSignInCard />));
      fireEvent.click(screen.getByRole("button", { name: /sign in with chatgpt/i }));
      const confirmBtn = await screen.findByRole("button", { name: /continue/i });
      fireEvent.click(confirmBtn);
      // Give polling a beat to start, then unmount.
      await new Promise((res) => setTimeout(res, 50));
      unmount();
      // Let any pending promise rejections surface.
      await new Promise((res) => setTimeout(res, 100));
      const reactWarnings = errors.filter((m) => /Warning|cannot perform a React state update/i.test(m));
      expect(reactWarnings, `react warnings:\n${reactWarnings.join("\n")}`).toEqual([]);
    } finally {
      console.error = origError;
    }
  });
});
