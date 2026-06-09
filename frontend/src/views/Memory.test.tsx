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

import { TeachFactForm, ReviseFactForm, parseMemoryJSON } from "@/views/Memory";

afterEach(cleanup);
beforeEach(() => {
  postJSON.mockReset();
  postJSON.mockResolvedValue({ id: "mem-1" });
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
    expect((screen.getByLabelText("Revise memory type") as HTMLSelectElement).value).toBe("PREFERENCE");
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
