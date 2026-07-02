// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { ExecutionProfilePicker } from "@/views/Chat";

const getJSON = vi.fn();
vi.mock("@/lib/api", async (orig) => {
  const actual = await orig<typeof import("@/lib/api")>();
  return { ...actual, getJSON: (...a: unknown[]) => getJSON(...a) };
});

beforeEach(() => {
  getJSON.mockReset();
  getJSON.mockResolvedValue({
    routable_run_profiles: ["local", "warden"],
    checks: [
      { profile_id: "local", status: "ok", detail: "direct host execution tools are routable" },
      { profile_id: "warden", status: "warning", detail: "requested namespace isolation is downgraded" },
    ],
  });
});
afterEach(cleanup);

describe("ExecutionProfilePicker", () => {
  it("selects a run execution profile", async () => {
    const onChange = vi.fn();
    render(<ExecutionProfilePicker value="" onChange={onChange} />);
    fireEvent.click(screen.getByLabelText("Choose execution profile"));
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/execution_profile_check"));
    fireEvent.click(screen.getByText("local"));
    expect(onChange).toHaveBeenCalledWith("local");
  });

  it("shows the selected profile label", () => {
    render(<ExecutionProfilePicker value="warden" onChange={() => {}} />);
    expect(screen.getByText("warden")).toBeTruthy();
  });

  it("offers dynamic profiles when the health report marks them routable", async () => {
    getJSON.mockResolvedValueOnce({
      routable_run_profiles: ["local", "warden", "docker", "ssh", "remote-agezt"],
      checks: [
        { profile_id: "docker", status: "ok", detail: "profile is routed and backend routing is available" },
        { profile_id: "ssh", status: "ok", detail: "profile is routed and backend routing is available" },
        { profile_id: "remote-agezt", status: "ok", detail: "whole-run peer delegation is routable" },
      ],
    });
    const onChange = vi.fn();
    render(<ExecutionProfilePicker value="" onChange={onChange} />);
    fireEvent.click(screen.getByLabelText("Choose execution profile"));
    await waitFor(() => expect(screen.getByText("docker")).toBeTruthy());
    expect(screen.getByText("ssh")).toBeTruthy();
    expect(screen.getByText("remote-agezt")).toBeTruthy();
    fireEvent.click(screen.getByText("docker"));
    expect(onChange).toHaveBeenCalledWith("docker");
  });
});
