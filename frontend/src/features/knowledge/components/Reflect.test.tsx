// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { Reflect } from "./Reflect";
import { UIProvider } from "@/components/ui/feedback";

const sampleMem = {
  audit: [
    {
      id: "m-1",
      ts: Date.now() - 1000,
      subject: "world",
      content: "I learned that the agent prefers plain JSON responses.",
    },
    {
      id: "m-2",
      ts: Date.now() - 2000,
      subject: "world",
      content: "Mistake — we should never write to /tmp directly.",
    },
  ],
};
const sampleJour = {
  events: [
    { ts: Date.now() - 1000, kind: "memory.write", actor: "agent", subject: "world" },
    { ts: Date.now() - 2000, kind: "policy.decision", actor: "governor", subject: "warden" },
    { ts: Date.now() - 3000, kind: "run.fail", actor: "agent", subject: "sched-1", note: "rolled back due to auth failure" },
  ],
};

vi.mock("@/app/api", () => ({
  getJSON: vi.fn().mockImplementation(async (url: string) => {
    if (url.startsWith("/api/memory/audit")) return sampleMem;
    if (url.startsWith("/api/journal")) return sampleJour;
    return {};
  }),
}));

function wrap(ui: React.ReactNode) {
  return render(<UIProvider>{ui}</UIProvider>);
}

afterEach(cleanup);

describe("Reflect (Knowledge › Thinking Partners)", () => {
  it("renders the heading and the lessons + corrections + reflective entries", async () => {
    wrap(<Reflect />);
    expect(screen.getByRole("heading", { level: 2, name: /Reflect/i })).toBeTruthy();

    await waitFor(() => {
      expect(screen.getByText(/prefers plain JSON responses/i)).toBeTruthy();
      expect(screen.getByText(/never write to \/tmp directly/i)).toBeTruthy();
    });

    // Both "Lessons written" and "Corrections" cards should be on the page
    expect(screen.getByText("Lessons written")).toBeTruthy();
    expect(screen.getByText("Corrections")).toBeTruthy();
    expect(screen.getByText("Reflective journal entries")).toBeTruthy();
  });
});
