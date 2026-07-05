// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { LoadMoreFooter } from "./load-more-footer";

afterEach(cleanup);

describe("LoadMoreFooter", () => {
  it("renders a load-more button with the page size when hasMore is true", () => {
    render(
      <LoadMoreFooter hasMore loadingMore={false} onLoadMore={() => {}} pageSize={50} label="tool log" />,
    );
    const btn = screen.getByRole("button", { name: /load 50 more/i });
    expect(btn).toBeTruthy();
    expect(btn.hasAttribute("disabled")).toBe(false);
  });

  it("fires onLoadMore when the button is clicked", () => {
    const onLoadMore = vi.fn();
    render(<LoadMoreFooter hasMore loadingMore={false} onLoadMore={onLoadMore} />);
    fireEvent.click(screen.getByRole("button"));
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });

  it("disables the button and sets aria-busy while loadingMore", () => {
    render(<LoadMoreFooter hasMore loadingMore onLoadMore={() => {}} />);
    const btn = screen.getByRole("button");
    expect(btn.hasAttribute("disabled")).toBe(true);
    expect(btn.getAttribute("aria-busy")).toBe("true");
    expect(screen.getByText(/loading/i)).toBeTruthy();
  });

  it("surfaces a moreError via role=alert without hiding the button", () => {
    render(
      <LoadMoreFooter hasMore loadingMore={false} moreError="boom" onLoadMore={() => {}} />,
    );
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toContain("boom");
    expect(screen.getByRole("button")).toBeTruthy();
  });

  it("shows a terminal marker (no button) once the list is drained", () => {
    render(<LoadMoreFooter hasMore={false} loadingMore={false} onLoadMore={() => {}} label="tool log" />);
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText(/end of tool log/i)).toBeTruthy();
  });

  it("defaults the terminal label to \"list\" when none is given", () => {
    render(<LoadMoreFooter hasMore={false} loadingMore={false} onLoadMore={() => {}} />);
    expect(screen.getByText(/end of list/i)).toBeTruthy();
  });
});
