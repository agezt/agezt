// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));
// Avoid the SSE EventSource (not in jsdom): stub the events hook.
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe: () => () => {} }),
}));

import { Providers } from "@/views/Providers";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/providers") return Promise.resolve({ routed: 0, fallbacks: 0, by_primary: {}, fallbacks_by_primary: {} });
    return Promise.resolve({ events: [] });
  });
});

describe("Providers reload (M745)", () => {
  it("Reload posts provider/reload and toasts the provider count", async () => {
    postAction.mockResolvedValue({ providers_reloaded: true, provider_count: 3 });
    render(withUI(<Providers />));
    await waitFor(() => expect(screen.getByRole("button", { name: /Reload/ })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Reload/ }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/provider/reload", {}));
    await waitFor(() => expect(screen.getByText(/Providers reloaded — 3 providers/)).toBeTruthy());
  });

  it("surfaces the daemon note when only the catalog refreshed", async () => {
    postAction.mockResolvedValue({ providers_reloaded: false, note: "OnReload not configured; restart the daemon." });
    render(withUI(<Providers />));
    await waitFor(() => expect(screen.getByRole("button", { name: /Reload/ })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Reload/ }));
    await waitFor(() => expect(screen.getByText(/OnReload not configured/)).toBeTruthy());
  });

  it("surfaces an error", async () => {
    postAction.mockRejectedValueOnce(new Error("reload boom"));
    render(withUI(<Providers />));
    await waitFor(() => expect(screen.getByRole("button", { name: /Reload/ })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Reload/ }));
    await waitFor(() => expect(screen.getByText(/reload boom/)).toBeTruthy());
  });
});
