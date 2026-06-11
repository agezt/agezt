import { describe, it, expect } from "vitest";
import { sessionsFromInboxThreads, lastSnippet, type InboxThread } from "./channelSessions";

describe("sessionsFromInboxThreads", () => {
  it("merges multiple correlations from the same user into one session", () => {
    const threads: InboxThread[] = [
      {
        correlation_id: "c1",
        channel_kind: "telegram",
        channel_id: "555",
        last_ts_unix_ms: 100,
        messages: [
          { direction: "in", sender: "alice", text: "hi", ts_unix_ms: 90 },
          { direction: "out", text: "hello!", ts_unix_ms: 100 },
        ],
      },
      {
        correlation_id: "c2",
        channel_kind: "telegram",
        channel_id: "555",
        last_ts_unix_ms: 200,
        messages: [
          { direction: "in", sender: "alice", text: "what's the weather", ts_unix_ms: 190 },
          { direction: "out", text: "sunny", ts_unix_ms: 200 },
        ],
      },
    ];
    const sessions = sessionsFromInboxThreads(threads);
    expect(sessions).toHaveLength(1);
    const s = sessions[0];
    expect(s.sender).toBe("alice");
    expect(s.channelKind).toBe("telegram");
    expect(s.correlationIds).toEqual(["c1", "c2"]);
    // Messages merged and time-ordered.
    expect(s.messages.map((m) => m.text)).toEqual(["hi", "hello!", "what's the weather", "sunny"]);
    expect(s.lastTs).toBe(200);
  });

  it("keeps different senders as separate sessions and sorts newest first", () => {
    const threads: InboxThread[] = [
      {
        correlation_id: "a",
        channel_kind: "telegram",
        channel_id: "1",
        last_ts_unix_ms: 50,
        messages: [{ direction: "in", sender: "bob", text: "old", ts_unix_ms: 50 }],
      },
      {
        correlation_id: "b",
        channel_kind: "telegram",
        channel_id: "1",
        last_ts_unix_ms: 300,
        messages: [{ direction: "in", sender: "carol", text: "new", ts_unix_ms: 300 }],
      },
    ];
    const sessions = sessionsFromInboxThreads(threads);
    expect(sessions).toHaveLength(2);
    expect(sessions[0].sender).toBe("carol"); // newest first
    expect(sessions[1].sender).toBe("bob");
  });

  it("lastSnippet marks outbound replies", () => {
    const [s] = sessionsFromInboxThreads([
      {
        correlation_id: "c",
        channel_kind: "telegram",
        channel_id: "1",
        messages: [
          { direction: "in", sender: "x", text: "ping", ts_unix_ms: 1 },
          { direction: "out", text: "pong reply", ts_unix_ms: 2 },
        ],
      },
    ]);
    expect(lastSnippet(s)).toBe("↩ pong reply");
  });
});
