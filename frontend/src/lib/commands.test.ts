import { describe, it, expect } from "vitest";
import { filterCommands, type CommandItem } from "@/lib/commands";

const items: CommandItem[] = [
  { id: "chat", label: "Chat", group: "View", run: () => {} },
  { id: "agents", label: "Agents", group: "View", run: () => {} },
  { id: "activity", label: "Activity", group: "View", run: () => {} },
  { id: "halt", label: "Halt all runs", group: "Action", keywords: "freeze stop", run: () => {} },
  { id: "stream", label: "Live Stream", group: "View", run: () => {} },
];

describe("filterCommands", () => {
  it("returns all items for an empty query, in order", () => {
    expect(filterCommands(items, "").map((i) => i.id)).toEqual(["chat", "agents", "activity", "halt", "stream"]);
  });

  it("prefers a label substring match", () => {
    const r = filterCommands(items, "agent");
    expect(r[0].id).toBe("agents");
  });

  it("matches keywords (freeze → Halt)", () => {
    const r = filterCommands(items, "freeze");
    expect(r[0].id).toBe("halt");
  });

  it("fuzzy-matches subsequences", () => {
    // "lstrm" is a subsequence of "live stream".
    const r = filterCommands(items, "lstrm");
    expect(r.map((i) => i.id)).toContain("stream");
  });

  it("drops non-matches", () => {
    expect(filterCommands(items, "zzzzz")).toEqual([]);
  });

  it("ranks an earlier substring higher", () => {
    const two: CommandItem[] = [
      { id: "a", label: "xxchat", group: "View", run: () => {} },
      { id: "b", label: "chatxx", group: "View", run: () => {} },
    ];
    expect(filterCommands(two, "chat")[0].id).toBe("b");
  });
});
