// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Backups } from "./Backups";
import { UIProvider } from "@/components/ui/feedback";

// The daemon's rollback row — the shape the console's rollback answer serves.
// The view reads THIS row, not a hand-written copy of it.
const sample = {
  checkpoints: [
    {
      id: "rb-abc123",
      kind: "config.setting",
      action: "config.set",
      run_id: "run-1",
      subject_id: "settings",
      subject_name: "agentzt/config.yaml",
      before_status: "paused",
      created_ms: Date.now() - 60_000,
    },
    {
      id: "rb-def456",
      kind: "skill.status",
      action: "skill.set",
      run_id: "run-2",
      subject_id: "skill-1",
      subject_name: "ops",
      created_ms: Date.now() - 120_000,
      applied_ms: Date.now() - 60_000,
    },
  ],
  count: 2,
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
    // The daemon's own fields: the target's name, not a description the
    // daemon never sends.
    expect(screen.getByText("agentzt/config.yaml")).toBeTruthy();
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

  it("leaves an already-restored checkpoint unrestorable — the daemon refuses it", async () => {
    wrap(<Backups />);
    await waitFor(() => {
      expect(screen.getByText("rb-def456")).toBeTruthy();
    });
    // The second checkpoint carries applied_ms: the daemon has already
    // restored it and answers "already applied" if asked again.
    const buttons = screen.getAllByRole("button", { name: /Restore/i }) as HTMLButtonElement[];
    expect(buttons.length).toBe(2);
    expect(buttons[0].disabled).toBe(false);
    expect(buttons[1].disabled).toBe(true);
  });
});
