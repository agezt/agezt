// @vitest-environment jsdom
//
// Tests for S4.1: connectionState classification logic. Pure-function tests
// don't need to mount the provider; we exercise the helper directly with
// several (connected, lastEventAt, now) triples.

import { describe, it, expect } from "vitest";
import { connectionState, STALE_MS } from "@/lib/events";

describe("connectionState", () => {
  it("reports disconnected when the socket is closed", () => {
    const r = connectionState({ connected: false, lastEventAt: null }, 1_000_000);
    expect(r.state).toBe("disconnected");
    expect(r.ageMs).toBeNull();
    expect(r.label).toMatch(/disconnected/);
  });

  it("reports disconnected with a measured age when the socket dropped after events", () => {
    const r = connectionState({ connected: false, lastEventAt: 1_000_000 - 4_000 }, 1_000_000);
    expect(r.state).toBe("disconnected");
    expect(r.ageMs).toBe(4_000);
    expect(r.label).toMatch(/4s ago/);
  });

  it("reports 'stale — no events yet' when connected but no event has arrived", () => {
    const r = connectionState({ connected: true, lastEventAt: null }, 1_000_000);
    expect(r.state).toBe("stale");
    expect(r.ageMs).toBeNull();
    expect(r.label).toMatch(/no events yet/);
  });

  it("reports 'live' when connected and a recent event exists (gap < STALE_MS)", () => {
    const now = 1_000_000;
    const r = connectionState({ connected: true, lastEventAt: now - 1_000 }, now);
    expect(r.state).toBe("live");
    expect(r.ageMs).toBe(1_000);
    expect(r.label).toMatch(/1s ago/);
  });

  it("reports 'live' at the boundary (gap === STALE_MS counts as live, > STALE_MS counts stale)", () => {
    const now = 1_000_000;
    expect(connectionState({ connected: true, lastEventAt: now - STALE_MS }, now).state).toBe("live");
    expect(connectionState({ connected: true, lastEventAt: now - (STALE_MS + 1) }, now).state).toBe("stale");
  });

  it("reports 'stale' when the gap is past the TTL", () => {
    const now = 1_000_000;
    const r = connectionState({ connected: true, lastEventAt: now - 30_000 }, now);
    expect(r.state).toBe("stale");
    expect(r.ageMs).toBe(30_000);
    expect(r.label).toMatch(/30s/);
  });

  it("clamps a tiny clock skew to a zero age rather than negative", () => {
    // Defence-in-depth: if the provider's lastEventAt is later than the
    // chip's nowMs (e.g. on a multi-tab browser where tabs can drift a
    // couple ms), we'd report a negative age. The classification should
    // still pick 'live' rather than 'stale'.
    const r = connectionState({ connected: true, lastEventAt: 1_000_005 }, 1_000_000);
    expect(r.state).toBe("live");
    expect(r.ageMs).toBeGreaterThanOrEqual(0);
  });
});
