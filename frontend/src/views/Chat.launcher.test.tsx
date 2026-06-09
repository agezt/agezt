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

import { PromptLauncher } from "@/views/Chat";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
});

describe("PromptLauncher", () => {
  it("renders nothing when there are no saved prompts", async () => {
    getJSON.mockResolvedValue({ prompts: [] });
    const { container } = render(<PromptLauncher onPick={() => {}} />);
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/prompts"));
    expect(container.querySelector("button")).toBeNull();
  });

  it("lists saved prompts and inserts the picked one's text", async () => {
    getJSON.mockResolvedValue({
      prompts: [
        { title: "Standup", text: "Summarize yesterday." },
        { title: "Review", text: "Review the diff." },
      ],
    });
    const onPick = vi.fn();
    render(<PromptLauncher onPick={onPick} />);
    await waitFor(() => expect(screen.getByLabelText("Insert a saved prompt")).toBeTruthy());

    fireEvent.click(screen.getByLabelText("Insert a saved prompt"));
    expect(screen.getByText("Standup")).toBeTruthy();
    expect(screen.getByText("Review")).toBeTruthy();

    fireEvent.click(screen.getByText("Review"));
    expect(onPick).toHaveBeenCalledWith("Review the diff.");
    // Menu closes after a pick.
    expect(screen.queryByText("Standup")).toBeNull();
  });
});
