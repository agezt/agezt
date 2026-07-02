// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { Server } from "lucide-react";
import { Page } from "@/components/ui/page";

afterEach(cleanup);

// The whole point of Page is scroll-safety: the default (scroll) mode must NOT
// pin the view to viewport height (no `h-full`), so a tall page grows and the
// app's <main> scrolls. fill mode is the opt-in bounded shell for real apps.
function root(container: HTMLElement): HTMLElement {
  return container.firstElementChild as HTMLElement;
}

describe("Page", () => {
  it("scroll mode grows (min-h-full, never h-full)", () => {
    const { container } = render(
      <Page title="Hello" icon={Server}>
        body
      </Page>,
    );
    const cls = root(container).className;
    expect(cls).toContain("min-h-full");
    expect(cls).not.toContain("h-full min-h-0");
  });

  it("fill mode is a bounded shell (h-full min-h-0)", () => {
    const { container } = render(
      <Page title="App" mode="fill">
        body
      </Page>,
    );
    const cls = root(container).className;
    expect(cls).toContain("h-full");
    expect(cls).toContain("min-h-0");
  });

  it("applies the readable width cap by default", () => {
    const { container } = render(<Page title="Form">body</Page>);
    expect(root(container).className).toContain("max-w-5xl");
  });

  it("full width drops the max-width cap", () => {
    const { container } = render(
      <Page title="Table" width="full">
        body
      </Page>,
    );
    const cls = root(container).className;
    expect(cls).not.toContain("max-w-5xl");
    expect(cls).not.toContain("max-w-[110rem]");
  });

  it("renders the header title, description and children", () => {
    render(
      <Page title="Execution Profiles" description="windows/amd64">
        <div>content-here</div>
      </Page>,
    );
    expect(screen.getByText("Execution Profiles")).toBeTruthy();
    expect(screen.getByText("windows/amd64")).toBeTruthy();
    expect(screen.getByText("content-here")).toBeTruthy();
  });

  it("omits the header when no title/icon/actions given", () => {
    render(
      <Page>
        <div>bare</div>
      </Page>,
    );
    // No <h2> header rendered, but children still there.
    expect(screen.getByText("bare")).toBeTruthy();
    expect(document.querySelector("h2")).toBeNull();
  });
});
