// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { Analyst } from "./Analyst";
import { UIProvider } from "@/components/ui/feedback";

const sampleEvents = [
  { ts: Date.now() - 1000, kind: "policy.decision", actor: "governor", subject: "warden" },
  { ts: Date.now() - 2000, kind: "provider.fallback", actor: "router", subject: "openai" },
  { ts: Date.now() - 3000, kind: "tool.invoke", actor: "agent", subject: "fs.read" },
  { ts: Date.now() - 4000, kind: "tool.invoke", actor: "agent", subject: "fs.write" },
  { ts: Date.now() - 5000, kind: "memory.write", actor: "agent", subject: "world" },
];

vi.mock("@/app/api", () => ({
  getJSON: vi.fn().mockImplementation(async (url: string) => {
    if (url.startsWith("/api/journal")) return { events: sampleEvents };
    return {};
  }),
}));

function wrap(ui: React.ReactNode) {
  return render(<UIProvider>{ui}</UIProvider>);
}

afterEach(cleanup);

describe("Analyst (Knowledge › Thinking Partners)", () => {
  it("renders the heading and the kind distribution from /api/journal", async () => {
    wrap(<Analyst />);
    expect(screen.getByRole("heading", { level: 2, name: /Analyst/i })).toBeTruthy();

    await waitFor(() => {
      // The table rows (td) carry the kind; the select options also do, but
      // getAllByText matches text in any element, so just assert presence
      // somewhere on the page.
      const txt = (() => document.body.textContent || "")();
      expect(txt).toMatch(/policy\.decision/);
      expect(txt).toMatch(/provider\.fallback/);
      expect(txt).toMatch(/tool\.invoke/);
    });

    expect((document.body.textContent || "").includes("memory.write")).toBe(true);
  });

  it("shows the top actors rail", async () => {
    wrap(<Analyst />);
    await waitFor(() => {
      expect(screen.getByText("Top actors")).toBeTruthy();
    });
    // actor "agent" appears 3 times (2 tool.invoke + 1 memory.write) so it's the top
    expect(screen.getAllByText("agent").length).toBeGreaterThan(0);
  });
});
