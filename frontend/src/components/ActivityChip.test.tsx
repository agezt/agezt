// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, fireEvent } from "@testing-library/react";
import { ActivityChip } from "@/components/ActivityChip";

afterEach(cleanup);

describe("ActivityChip", () => {
  it("is hidden when nothing is in flight", () => {
    render(<ActivityChip count={0} />);
    expect(screen.queryByText("working")).toBeNull();
  });

  it("lights up with a count when runs are in flight", () => {
    render(<ActivityChip count={3} />);
    expect(screen.getByText("working")).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy();
    // announced for assistive tech as background work, not a frozen screen
    expect(screen.getByLabelText(/arka planda çalışıyor/)).toBeTruthy();
  });

  it("caps the displayed count at 99+", () => {
    render(<ActivityChip count={250} />);
    expect(screen.getByText("99+")).toBeTruthy();
  });

  it("invokes onClick (jump to the Overseer)", () => {
    const onClick = vi.fn();
    render(<ActivityChip count={1} onClick={onClick} />);
    fireEvent.click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("keeps glowing for a beat after the work finishes (afterglow), then hides", () => {
    vi.useFakeTimers();
    try {
      const { rerender } = render(<ActivityChip count={2} />);
      expect(screen.getByText("working")).toBeTruthy();
      // Work finishes — chip must NOT vanish instantly (would flicker past).
      rerender(<ActivityChip count={0} />);
      expect(screen.getByText("working")).toBeTruthy();
      // After the afterglow window it clears.
      act(() => {
        vi.advanceTimersByTime(1600);
      });
      expect(screen.queryByText("working")).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });
});
