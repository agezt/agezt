// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { Inbox } from "lucide-react";
import { EmptyState } from "@/components/ui/empty";

afterEach(cleanup);

describe("EmptyState", () => {
  it("renders the title and hint in a status region", () => {
    render(<EmptyState icon={Inbox} title="Nothing here" hint="Add something to get started." />);
    const region = screen.getByRole("status");
    expect(region.textContent).toContain("Nothing here");
    expect(region.textContent).toContain("Add something to get started.");
  });

  it("omits the hint paragraph when none is given", () => {
    render(<EmptyState icon={Inbox} title="Empty" />);
    expect(screen.getByText("Empty")).toBeTruthy();
  });

  it("renders an action node when provided", () => {
    render(<EmptyState icon={Inbox} title="Empty" action={<button>Do it</button>} />);
    expect(screen.getByText("Do it")).toBeTruthy();
  });
});
