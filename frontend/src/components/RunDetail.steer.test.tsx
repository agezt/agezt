// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const postAction = vi.fn();
vi.mock("@/lib/api", () => ({ postAction: (...a: unknown[]) => postAction(...a) }));

import { SteerControls } from "@/components/RunDetail";
import type { AgentEvent } from "@/lib/events";

const liveArc: AgentEvent[] = [{ kind: "task.received", correlation_id: "run-7", payload: {} }];

beforeEach(() => {
  postAction.mockReset();
  postAction.mockResolvedValue({});
});
afterEach(cleanup);

describe("SteerControls cancel (M908)", () => {
  it("arms on first click and cancels the run by correlation id on the second", async () => {
    render(<SteerControls correlationId="run-7" arc={liveArc} />);
    const btn = () => screen.getByRole("button", { name: /Cancel|Confirm cancel/ });

    // First click arms the confirm — no cancel issued yet.
    fireEvent.click(btn());
    expect(screen.getByRole("button", { name: /Confirm cancel/ })).toBeTruthy();
    expect(postAction).not.toHaveBeenCalled();

    // Second click fires the targeted cancel.
    fireEvent.click(btn());
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/cancel_run", { correlation: "run-7" }),
    );
  });

  it("does not steer or pause when cancelling (cancel is its own action)", async () => {
    render(<SteerControls correlationId="run-7" arc={liveArc} />);
    fireEvent.click(screen.getByRole("button", { name: /^Cancel$/ }));
    fireEvent.click(screen.getByRole("button", { name: /Confirm cancel/ }));
    await waitFor(() => expect(postAction).toHaveBeenCalledTimes(1));
    expect(postAction).toHaveBeenCalledWith("/api/cancel_run", { correlation: "run-7" });
  });
});
