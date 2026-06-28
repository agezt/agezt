// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  authHeaders: (h?: Record<string, string>) => h || {},
}));

const streamInstall = vi.fn();
vi.mock("@/lib/toolbox", async () => {
  const actual = await vi.importActual<typeof import("@/lib/toolbox")>("@/lib/toolbox");
  return {
    ...actual,
    streamInstall: (...a: unknown[]) => streamInstall(...a),
  };
});

import { Toolbox } from "@/views/Toolbox";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

beforeEach(() => {
  getJSON.mockReset().mockResolvedValue({
    os: "windows",
    managers: ["winget"],
    installed_count: 0,
    missing_count: 1,
    tools: [
      {
        name: "ripgrep",
        category: "search",
        description: "Fast text search",
        installed: false,
        installable: true,
        manager: "winget",
        command: "winget install BurntSushi.ripgrep.MSVC",
      },
    ],
  });
  streamInstall.mockReset().mockImplementation(async (_names, onFrame) => {
    onFrame({
      kind: "toolbox.progress",
      payload: { tool: "ripgrep", ok: true, version: "14.1.1", manager: "winget" },
    });
  });
});
afterEach(cleanup);

describe("Toolbox", () => {
  it("shows install progress in a modal instead of an inline page panel", async () => {
    render(withUI(<Toolbox />));
    await screen.findByText("ripgrep");

    const installButtons = screen.getAllByRole("button", { name: /install/i });
    fireEvent.click(installButtons[installButtons.length - 1]);

    const dialog = await screen.findByRole("dialog", { name: "Install output" });
    expect(dialog).toBeTruthy();
    expect(await within(dialog).findByText("14.1.1")).toBeTruthy();
    expect(within(dialog).getByText("winget")).toBeTruthy();
    await waitFor(() => expect(streamInstall).toHaveBeenCalledWith(["ripgrep"], expect.any(Function), expect.any(AbortSignal)));
  });
});
