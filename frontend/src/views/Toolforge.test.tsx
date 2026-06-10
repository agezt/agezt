// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { Toolforge, NewToolForm, toolNameOk, statusBadge } from "@/views/Toolforge";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  postJSON.mockResolvedValue({});
  postAction.mockResolvedValue({});
});

describe("toolNameOk", () => {
  it("mirrors the kernel toolforge name rule", () => {
    for (const s of ["fetch_weather", "a", "x9", "tool_2_v3"]) expect(toolNameOk(s)).toBe(true);
    for (const s of ["", "Fetch", "has space", "9tool", "_lead", "kebab-case", "a".repeat(41)])
      expect(toolNameOk(s)).toBe(false);
  });
});

describe("statusBadge", () => {
  it("maps live to good, the kill switch to bad, drafts to neutral", () => {
    expect(statusBadge("active")).toBe("good");
    expect(statusBadge("quarantined")).toBe("bad");
    expect(statusBadge("draft")).toBe("default");
    expect(statusBadge(undefined)).toBe("default");
  });
});

describe("NewToolForm", () => {
  it("disables Draft until name+description+code are present, then posts the tool shape", async () => {
    const onCreated = vi.fn();
    render(<NewToolForm onCreated={onCreated} onError={() => {}} />);
    const draft = () => screen.getByRole("button", { name: /Draft tool/ }) as HTMLButtonElement;
    expect(draft().disabled).toBe(true);

    fireEvent.change(screen.getByLabelText("Tool name"), { target: { value: "BAD NAME" } });
    fireEvent.change(screen.getByLabelText("Tool description"), { target: { value: "weather" } });
    fireEvent.change(screen.getByLabelText("Tool code"), { target: { value: "print(1)" } });
    expect(draft().disabled).toBe(true); // name still invalid

    fireEvent.change(screen.getByLabelText("Tool name"), { target: { value: "fetch_weather" } });
    expect(draft().disabled).toBe(false);
    fireEvent.click(draft());

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/toolforge/draft",
        expect.objectContaining({
          tool: expect.objectContaining({
            name: "fetch_weather",
            description: "weather",
            language: "python",
            code: "print(1)",
          }),
        }),
      ),
    );
    expect(onCreated).toHaveBeenCalledWith("fetch_weather");
  });
});

describe("Toolforge", () => {
  const twoTools = {
    tools: [
      {
        id: "01A",
        name: "fetch_weather",
        description: "weather lookup",
        language: "python",
        status: "active",
        tested_ok: true,
        callable_as: "forge_fetch_weather",
      },
      { id: "01B", name: "scraper", language: "node", status: "draft", tested_ok: false },
    ],
    count: 2,
    active_count: 1,
  };

  it("renders tools from /api/toolforge with status, tested state, and callable name", async () => {
    getJSON.mockResolvedValue(twoTools);
    render(withUI(<Toolforge />));
    await waitFor(() => expect(screen.getByText("fetch_weather")).toBeTruthy());
    expect(screen.getByText("scraper")).toBeTruthy();
    expect(screen.getByText("active")).toBeTruthy();
    expect(screen.getByText("untested")).toBeTruthy();
    expect(screen.getByText("forge_fetch_weather")).toBeTruthy();
    expect(getJSON).toHaveBeenCalledWith("/api/toolforge");
  });

  it("keeps Promote disabled for an untested draft (only tested code goes live)", async () => {
    getJSON.mockResolvedValue(twoTools);
    render(withUI(<Toolforge />));
    await waitFor(() => expect(screen.getByText("scraper")).toBeTruthy());
    const promote = screen.getByRole("button", { name: "Promote scraper" }) as HTMLButtonElement;
    expect(promote.disabled).toBe(true);
    // The live tool shows Quarantine instead of Promote.
    expect(screen.queryByRole("button", { name: "Promote fetch_weather" })).toBeNull();
    expect(screen.getByRole("button", { name: "Quarantine fetch_weather" })).toBeTruthy();
  });

  it("running a test posts /api/toolforge/test and shows the verdict + output", async () => {
    getJSON.mockResolvedValue(twoTools);
    postAction.mockResolvedValue({ ok: true, output: "weather for izmir: sunny" });
    render(withUI(<Toolforge />));
    await waitFor(() => expect(screen.getByText("scraper")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "Test scraper" }));
    fireEvent.change(screen.getByLabelText("Test input for scraper"), {
      target: { value: '{"city":"izmir"}' },
    });
    fireEvent.click(screen.getByRole("button", { name: "Run test for scraper" }));

    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/toolforge/test", {
        ref: "scraper",
        input: '{"city":"izmir"}',
      }),
    );
    await waitFor(() => expect(screen.getByText("PASS")).toBeTruthy());
    expect(screen.getByText("weather for izmir: sunny")).toBeTruthy();
  });

  it("editing fetches the full record (the list strips code) and posts the edit", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/toolforge/show"
        ? Promise.resolve({
            tool: {
              id: "01B", name: "scraper", language: "node", status: "draft",
              description: "scrapes", code: "console.log(1)",
            },
          })
        : Promise.resolve(twoTools),
    );
    render(withUI(<Toolforge />));
    await waitFor(() => expect(screen.getByText("scraper")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "Edit scraper" }));
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/toolforge/show", { ref: "scraper" }));
    const code = (await screen.findByLabelText("Tool code")) as HTMLTextAreaElement;
    expect(code.value).toBe("console.log(1)");

    fireEvent.change(code, { target: { value: "console.log(2)" } });
    fireEvent.click(screen.getByRole("button", { name: /Save/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/toolforge/edit",
        expect.objectContaining({
          ref: "scraper",
          tool: expect.objectContaining({ code: "console.log(2)" }),
        }),
      ),
    );
  });

  it("shows the empty state when the forge is empty", async () => {
    getJSON.mockResolvedValue({ tools: [], count: 0, active_count: 0 });
    render(withUI(<Toolforge />));
    await waitFor(() => expect(screen.getByText("No script tools yet")).toBeTruthy());
  });
});
