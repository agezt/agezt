// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: vi.fn(),
}));

import { ApprovalsHistory } from "@/views/Approvals";

afterEach(cleanup);
beforeEach(() => getJSON.mockReset());

describe("ApprovalsHistory (M773)", () => {
  it("lists resolved decisions with their status, and hides still-pending rows", async () => {
    getJSON.mockResolvedValue({
      approvals: [
        { approval_id: "a1", capability: "shell", reason: "rm -rf /tmp/x", status: "denied", resolved_by: "operator", ts_unix_ms: 3 },
        { approval_id: "a2", capability: "http", reason: "POST to api.example.com", status: "granted", resolved_by: "operator", ts_unix_ms: 2 },
        { approval_id: "a3", capability: "file", reason: "write config", status: "timeout", ts_unix_ms: 1 },
        { approval_id: "a4", capability: "web_search", reason: "still waiting", status: "pending", ts_unix_ms: 4 },
      ],
    });
    render(<ApprovalsHistory />);
    await waitFor(() => expect(screen.getByText("granted")).toBeTruthy());
    expect(screen.getByText("denied")).toBeTruthy();
    expect(screen.getByText("timeout")).toBeTruthy();
    // The pending row is excluded (it lives in the panel above).
    expect(screen.queryByText("still waiting")).toBeNull();
    // Count reflects only the 3 resolved rows.
    expect(screen.getByText("(3)")).toBeTruthy();
    expect(screen.getByText(/rm -rf/)).toBeTruthy();
    expect(screen.getAllByText("by operator").length).toBe(2);
  });

  it("shows a graceful empty state when nothing has been resolved", async () => {
    getJSON.mockResolvedValue({ approvals: [] });
    render(<ApprovalsHistory />);
    await waitFor(() => expect(screen.getByText(/no resolved approvals yet/)).toBeTruthy());
  });

  it("requests the log with a limit", async () => {
    getJSON.mockResolvedValue({ approvals: [] });
    render(<ApprovalsHistory />);
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/approvals_log", { limit: "50" }));
  });
});
