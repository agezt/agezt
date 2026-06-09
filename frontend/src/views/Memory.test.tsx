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

import { TeachFactForm } from "@/views/Memory";

afterEach(cleanup);
beforeEach(() => {
  postJSON.mockReset();
  postJSON.mockResolvedValue({ id: "mem-1" });
});

describe("TeachFactForm", () => {
  it("disables the button until content is entered", () => {
    render(<TeachFactForm onAdded={() => {}} onError={() => {}} />);
    expect((screen.getByRole("button", { name: /Remember it/ }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Memory content"), { target: { value: "Owner is in Istanbul" } });
    expect((screen.getByRole("button", { name: /Remember it/ }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("posts a fact with subject, content (trimmed) and default type FACT", async () => {
    const onAdded = vi.fn();
    render(<TeachFactForm onAdded={onAdded} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Memory subject"), { target: { value: "  Timezone  " } });
    fireEvent.change(screen.getByLabelText("Memory content"), { target: { value: "  UTC+3  " } });
    fireEvent.click(screen.getByRole("button", { name: /Remember it/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/memory/add", { content: "UTC+3", subject: "Timezone", type: "FACT" }),
    );
    await waitFor(() => expect(onAdded).toHaveBeenCalledWith("Timezone"));
  });

  it("honours a chosen type (preference)", async () => {
    render(<TeachFactForm onAdded={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Memory content"), { target: { value: "terse replies" } });
    fireEvent.change(screen.getByLabelText("Memory type"), { target: { value: "PREFERENCE" } });
    fireEvent.click(screen.getByRole("button", { name: /Remember it/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/memory/add", { content: "terse replies", subject: "", type: "PREFERENCE" }),
    );
  });

  it("surfaces an error", async () => {
    postJSON.mockRejectedValueOnce(new Error("nope"));
    const onError = vi.fn();
    render(<TeachFactForm onAdded={() => {}} onError={onError} />);
    fireEvent.change(screen.getByLabelText("Memory content"), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: /Remember it/ }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith("nope"));
  });
});
