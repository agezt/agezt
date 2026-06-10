// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));

import { AuthGate, Login } from "@/views/Login";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
});

describe("AuthGate", () => {
  it("renders children when no password is required", async () => {
    getJSON.mockResolvedValue({ password_required: false, authed: true });
    render(
      <AuthGate>
        <div>app body</div>
      </AuthGate>,
    );
    await waitFor(() => expect(screen.getByText("app body")).toBeTruthy());
  });

  it("renders children when the probe fails (never locks out a usable console)", async () => {
    getJSON.mockRejectedValue(new Error("boom"));
    render(
      <AuthGate>
        <div>app body</div>
      </AuthGate>,
    );
    await waitFor(() => expect(screen.getByText("app body")).toBeTruthy());
  });

  it("shows the lock screen — not the app — when a password is required and not yet authed", async () => {
    getJSON.mockResolvedValue({ password_required: true, authed: false });
    render(
      <AuthGate>
        <div>app body</div>
      </AuthGate>,
    );
    await waitFor(() => expect(screen.getByText("Console locked")).toBeTruthy());
    expect(screen.queryByText("app body")).toBeNull();
  });

  it("reveals the app after a successful login", async () => {
    getJSON.mockResolvedValue({ password_required: true, authed: false });
    postJSON.mockResolvedValue({ ok: true });
    render(
      <AuthGate>
        <div>app body</div>
      </AuthGate>,
    );
    await waitFor(() => expect(screen.getByText("Console locked")).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Console password"), { target: { value: "hunter2" } });
    fireEvent.click(screen.getByRole("button", { name: "Unlock console" }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/login", { password: "hunter2" }));
    await waitFor(() => expect(screen.getByText("app body")).toBeTruthy());
  });
});

describe("Login", () => {
  it("surfaces the error on a wrong password and does not advance", async () => {
    postJSON.mockRejectedValue(new Error("invalid password"));
    const onAuthed = vi.fn();
    render(<Login onAuthed={onAuthed} />);
    fireEvent.change(screen.getByLabelText("Console password"), { target: { value: "wrong" } });
    fireEvent.click(screen.getByRole("button", { name: "Unlock console" }));
    await waitFor(() => expect(screen.getByRole("alert").textContent).toContain("invalid password"));
    expect(onAuthed).not.toHaveBeenCalled();
  });
});
