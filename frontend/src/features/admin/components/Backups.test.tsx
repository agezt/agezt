// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Backups } from "./Backups";
import { UIProvider } from "@/components/ui/feedback";

const sample = {
  checkpoints: [
    {
      id: "rb-abc123",
      created_at: Date.now() - 60_000,
      run_id: "run-1",
      kind: "config",
      description: "before settings write",
      affected_paths: ["/etc/agentzt/config.yaml", "/etc/agentzt/skills/"],
      size_bytes: 4096,
      reversible: true,
    },
    {
      id: "rb-def456",
      created_at: Date.now() - 120_000,
      run_id: "run-2",
      kind: "skill",
      description: "before skill import",
      affected_paths: ["/etc/agentzt/skills/new.md"],
      size_bytes: 1024,
      reversible: true,
    },
  ],
};

vi.mock("@/app/api", () => ({
  getJSON: vi.fn().mockImplementation(async () => sample),
  postJSON: vi.fn().mockResolvedValue({ ok: true }),
}));

function wrap(ui: React.ReactNode) {
  return render(<UIProvider>{ui}</UIProvider>);
}

afterEach(cleanup);

describe("Backups (Admin)", () => {
  it("renders the heading and lists checkpoints from /api/rollback/checkpoints", async () => {
    wrap(<Backups />);
    expect(screen.getByRole("heading", { level: 2, name: /Backups/i })).toBeTruthy();

    await waitFor(() => {
      expect(screen.getByText("rb-abc123")).toBeTruthy();
      expect(screen.getByText("rb-def456")).toBeTruthy();
    });
    expect(screen.getByText("before settings write")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: /Restore/i }).length).toBeGreaterThan(0);
  });

  it("opens a confirm dialog when Restore is clicked", async () => {
    wrap(<Backups />);
    await waitFor(() => {
      expect(screen.getByText("rb-abc123")).toBeTruthy();
    });
    fireEvent.click(screen.getAllByRole("button", { name: /Restore/i })[0]);
    await waitFor(() => {
      expect(screen.getByText(/Restore checkpoint\?/i)).toBeTruthy();
    });
  });
});
