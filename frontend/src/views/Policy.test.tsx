// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { DenyAddForm, PolicyTestForm } from "@/views/Policy";

afterEach(cleanup);
beforeEach(() => {
  postAction.mockReset();
  postAction.mockResolvedValue({ ok: true });
  getJSON.mockReset();
});

describe("PolicyTestForm (M753)", () => {
  it("probes the chosen capability + input and shows an ALLOW verdict with level", async () => {
    getJSON.mockResolvedValue({ decision: "allow", capability: "shell.exec", level: "L4", reason: "trusted" });
    render(<PolicyTestForm capabilities={["shell.exec", "code_exec"]} />);
    fireEvent.change(screen.getByLabelText("Test input"), { target: { value: "echo hi" } });
    fireEvent.click(screen.getByRole("button", { name: /Test/ }));
    await waitFor(() =>
      expect(getJSON).toHaveBeenCalledWith("/api/edict/test", { capability: "shell.exec", input: "echo hi" }),
    );
    await waitFor(() => expect(screen.getByText("ALLOW")).toBeTruthy());
    expect(screen.getByText("L4")).toBeTruthy();
  });

  it("renders a hard DENY with the matching rule name", async () => {
    getJSON.mockResolvedValue({
      decision: "deny",
      level: "L0",
      hard_denied: true,
      hard_deny_rule: "runtime0",
      reason: "blocked by runtime0",
    });
    render(<PolicyTestForm capabilities={["code_exec"]} />);
    fireEvent.change(screen.getByLabelText("Test input"), { target: { value: "rm -rf /" } });
    fireEvent.click(screen.getByRole("button", { name: /Test/ }));
    await waitFor(() => expect(screen.getByText("DENY · hard")).toBeTruthy());
    expect(screen.getByText("runtime0")).toBeTruthy();
  });

  it("shows ASK when the call would pause for approval (allow + requires_approval)", async () => {
    getJSON.mockResolvedValue({ decision: "deny", level: "L1", requires_approval: true, would_ask: true });
    render(<PolicyTestForm capabilities={["shell.exec"]} />);
    fireEvent.click(screen.getByRole("button", { name: /Test/ }));
    await waitFor(() => expect(screen.getByText("ASK")).toBeTruthy());
  });

  it("surfaces a probe error", async () => {
    getJSON.mockRejectedValueOnce(new Error("bad capability"));
    render(<PolicyTestForm capabilities={["shell.exec"]} />);
    fireEvent.click(screen.getByRole("button", { name: /Test/ }));
    await waitFor(() => expect(screen.getByText("bad capability")).toBeTruthy());
  });
});

describe("DenyAddForm", () => {
  it("disables Add until a substring is entered", () => {
    render(<DenyAddForm capabilities={["shell.exec"]} onAdded={() => {}} onError={() => {}} />);
    expect((screen.getByRole("button", { name: /Add/ }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Deny rule substring"), { target: { value: "rm -rf" } });
    expect((screen.getByRole("button", { name: /Add/ }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("posts an all-capabilities rule (bare substring)", async () => {
    const onAdded = vi.fn();
    render(<DenyAddForm capabilities={["shell.exec"]} onAdded={onAdded} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Deny rule substring"), { target: { value: "  rm -rf  " } });
    fireEvent.click(screen.getByRole("button", { name: /Add/ }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/edict/deny_add", { rule: "rm -rf" }));
    await waitFor(() => expect(onAdded).toHaveBeenCalledWith("rm -rf"));
  });

  it("scopes the rule to a capability (cap:substring)", async () => {
    render(<DenyAddForm capabilities={["shell.exec", "http.fetch"]} onAdded={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Deny rule substring"), { target: { value: "curl" } });
    fireEvent.change(screen.getByLabelText("Deny rule capability scope"), { target: { value: "shell.exec" } });
    fireEvent.click(screen.getByRole("button", { name: /Add/ }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/edict/deny_add", { rule: "shell.exec:curl" }));
  });

  it("surfaces an error", async () => {
    postAction.mockRejectedValueOnce(new Error("bad rule"));
    const onError = vi.fn();
    render(<DenyAddForm capabilities={[]} onAdded={() => {}} onError={onError} />);
    fireEvent.change(screen.getByLabelText("Deny rule substring"), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: /Add/ }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith("bad rule"));
  });
});
