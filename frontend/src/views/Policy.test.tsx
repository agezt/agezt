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

import { DenyAddForm } from "@/views/Policy";

afterEach(cleanup);
beforeEach(() => {
  postAction.mockReset();
  postAction.mockResolvedValue({ ok: true });
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
