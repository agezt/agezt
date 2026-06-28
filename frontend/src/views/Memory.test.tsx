// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor, within } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

const toast = vi.fn();
const confirm = vi.fn();
vi.mock("@/components/ui/feedback", () => ({
  useUI: () => ({ confirm: (...a: unknown[]) => confirm(...a), toast }),
}));

import { Memory, TeachFactForm, ReviseFactForm, parseMemoryJSON } from "@/views/Memory";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
  toast.mockReset();
  confirm.mockReset();
  postJSON.mockReset();
  postJSON.mockResolvedValue({ id: "mem-1" });
});

describe("Operator Profile card (M1000)", () => {
  it("renders profile facets and rebuilds on demand", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/memory")
        return Promise.resolve({
          records: [
            { id: "p1", subject: "operator profile: expertise", content: "Go and React.", type: "PREFERENCE", tags: { source: "profile" } },
            { id: "m1", subject: "kubernetes", content: "runs in frankfurt", type: "FACT" },
          ],
        });
      return Promise.resolve(null); // audit
    });
    postAction.mockResolvedValue({ facets_written: 1, input_records: 5 });
    render(<Memory />);

    // The learned facet shows under the profile card (capitalized facet name).
    await waitFor(() => expect(screen.getByText(/Go and React\./)).toBeTruthy());
    expect(screen.getByText("expertise")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /rebuild/i }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/profile/rebuild", {}));
    await waitFor(() => expect(toast).toHaveBeenCalledWith(expect.stringMatching(/rebuilt: 1 facet/i), "success"));
  });
});

describe("parseMemoryJSON (M750)", () => {
  const rec = (content: string) => ({ content, subject: "tz", type: "FACT" });

  it("reads a bare array, a {memory:[…]} and a {records:[…]} wrapper", () => {
    expect(parseMemoryJSON(JSON.stringify([rec("a")]))).toHaveLength(1);
    expect(parseMemoryJSON(JSON.stringify({ memory: [rec("b")] }))).toHaveLength(1);
    expect(parseMemoryJSON(JSON.stringify({ version: 1, records: [rec("c")] }))).toHaveLength(1);
  });

  it("keeps content + optional subject/type/confidence, dropping kernel fields", () => {
    const out = parseMemoryJSON(
      JSON.stringify([
        {
          id: "m-1",
          created_ms: 1,
          last_seen_ms: 2,
          source_event: "x",
          tags: { source: "agent" },
          content: "  Owner is in Istanbul  ",
          subject: "  Owner timezone  ",
          type: "preference",
          confidence: 0.8,
        },
      ]),
    );
    expect(out[0]).toEqual({ content: "Owner is in Istanbul", subject: "Owner timezone", type: "PREFERENCE", confidence: 0.8 });
    expect(out[0]).not.toHaveProperty("id");
    expect(out[0]).not.toHaveProperty("tags");
  });

  it("omits an empty/zero confidence and a blank subject/type", () => {
    const out = parseMemoryJSON(JSON.stringify([{ content: "bare fact", subject: "  ", type: "", confidence: 0 }]));
    expect(out[0]).toEqual({ content: "bare fact" });
  });

  it("drops entries with no content", () => {
    const out = parseMemoryJSON(JSON.stringify([{ content: "keep" }, { subject: "no content" }, { content: "   " }]));
    expect(out).toHaveLength(1);
    expect(out[0].content).toBe("keep");
  });

  it("throws on invalid JSON, a non-array shape, or nothing valid", () => {
    expect(() => parseMemoryJSON("nope")).toThrow();
    expect(() => parseMemoryJSON('{"foo":1}')).toThrow(/expected an array/);
    expect(() => parseMemoryJSON("[{}]")).toThrow(/no valid memories/);
  });
});

describe("Memory scopes (M915)", () => {
  const records = [
    { id: "m-shared", subject: "deploy-url", content: "example.com", type: "FACT", tags: { source: "operator" } },
    { id: "m-priv", subject: "draft", content: "research notes", type: "FACT", tags: { source: "agent", scope: "researcher" } },
  ];

  it("renders scope filter chips and filters to one agent's private notes", async () => {
    getJSON.mockResolvedValue({ records });
    render(<Memory />);
    await screen.findByText("deploy-url");

    // Chips: All / Shared / researcher, with counts.
    const group = screen.getByRole("group", { name: /scope filter/i });
    expect(group).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /researcher/ }));
    expect(screen.queryByText("deploy-url")).toBeNull();
    expect(screen.getByText("draft")).toBeTruthy();

    // Clicking the active chip clears the filter.
    fireEvent.click(screen.getByRole("button", { name: /researcher/ }));
    expect(screen.getByText("deploy-url")).toBeTruthy();
  });

  it("promotes a private record to shared after confirm", async () => {
    getJSON.mockResolvedValue({ records });
    confirm.mockResolvedValue(true);
    postAction.mockResolvedValue({ promoted: true });
    render(<Memory />);
    await screen.findByText("draft");

    fireEvent.click(screen.getByTitle(/promote to shared memory/i));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/memory/promote", { id: "m-priv" }));
  });

  it("shared records get no promote button and no chips when nothing is scoped", async () => {
    getJSON.mockResolvedValue({ records: [records[0]] });
    render(<Memory />);
    await screen.findByText("deploy-url");
    expect(screen.queryByTitle(/promote to shared memory/i)).toBeNull();
    expect(screen.queryByRole("group", { name: /scope filter/i })).toBeNull();
  });
});

describe("Memory hygiene", () => {
  it("loads and renders the audit summary", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/memory/audit"
        ? Promise.resolve({ usable: 2, expired: 1, suspended: 0, contradiction_load: 1 })
        : Promise.resolve({ records: [{ id: "m1", subject: "project", content: "Agezt uses Go", type: "FACT" }] }),
    );
    render(<Memory />);
    await screen.findByText("project");
    // Audit summary moved into metric widgets (labels lost their trailing colon).
    expect(screen.getAllByText(/usable/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/expired/i)).toBeTruthy();
    expect(screen.getByText(/conflict load/i)).toBeTruthy();
  });

  it("cleans low-value memories through dry-run then execute", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/memory/audit" ? Promise.resolve({ usable: 1, expired: 0, suspended: 0, contradiction_load: 0 }) : Promise.resolve({ records: [] }),
    );
    postAction.mockImplementation((path: string, body: any) => {
      if (path === "/api/memory/clean" && body?.dry_run === "true") return Promise.resolve({ rejected: 2, scanned: 4 });
      if (path === "/api/memory/clean" && body?.dry_run === "false") return Promise.resolve({ removed: 2 });
      return Promise.resolve({});
    });
    confirm.mockResolvedValue(true);
    render(<Memory />);
    await screen.findByText(/No memories yet/i);

    fireEvent.click(screen.getByRole("button", { name: /Clean/i }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/memory/clean", { dry_run: "true" }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/memory/clean", { dry_run: "false" }));
    expect(confirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: expect.stringMatching(/Clean low-value/),
        message: expect.stringMatching(/permanently deleted/),
      }),
    );
  });

  it("does not ask for confirmation when clean dry-run finds nothing", async () => {
    getJSON.mockImplementation((path: string) =>
      path === "/api/memory/audit" ? Promise.resolve({ usable: 0, expired: 0, suspended: 0, contradiction_load: 0 }) : Promise.resolve({ records: [] }),
    );
    postAction.mockResolvedValueOnce({ rejected: 0, scanned: 0 });
    render(<Memory />);
    await screen.findByText(/No memories yet/i);

    fireEvent.click(screen.getByRole("button", { name: /Clean/i }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/memory/clean", { dry_run: "true" }));
    expect(confirm).not.toHaveBeenCalled();
  });
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
    fireEvent.click(within(screen.getByRole("group", { name: "Memory type" })).getByRole("button", { name: /preference/i }));
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

describe("ReviseFactForm (M731)", () => {
  const record = {
    id: "mem-9",
    type: "preference",
    subject: "Owner tz",
    content: "Owner is in UTC",
    confidence: 0.9,
  };

  it("prefills subject, content and type (upper-cased) from the record", () => {
    render(<ReviseFactForm record={record} onRevised={() => {}} onError={() => {}} />);
    expect((screen.getByLabelText("Revise memory subject") as HTMLInputElement).value).toBe("Owner tz");
    expect((screen.getByLabelText("Revise memory content") as HTMLTextAreaElement).value).toBe("Owner is in UTC");
    expect(
      within(screen.getByRole("group", { name: "Revise memory type" }))
        .getByRole("button", { name: /preference/i })
        .getAttribute("aria-pressed"),
    ).toBe("true");
  });

  it("posts memory/supersede with old_id, trimmed content, and the carried confidence", async () => {
    const onRevised = vi.fn();
    render(<ReviseFactForm record={record} onRevised={onRevised} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Revise memory content"), { target: { value: "  Owner is in Istanbul, UTC+3  " } });
    fireEvent.click(screen.getByRole("button", { name: /Save revision/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/memory/supersede", {
        old_id: "mem-9",
        content: "Owner is in Istanbul, UTC+3",
        subject: "Owner tz",
        type: "PREFERENCE",
        confidence: 0.9,
      }),
    );
    await waitFor(() => expect(onRevised).toHaveBeenCalled());
  });

  it("disables Save when content is cleared", () => {
    render(<ReviseFactForm record={record} onRevised={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Revise memory content"), { target: { value: "   " } });
    expect((screen.getByRole("button", { name: /Save revision/ }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("surfaces a revise error", async () => {
    postJSON.mockRejectedValueOnce(new Error("boom"));
    const onError = vi.fn();
    render(<ReviseFactForm record={record} onRevised={() => {}} onError={onError} />);
    fireEvent.click(screen.getByRole("button", { name: /Save revision/ }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith("boom"));
  });
});
