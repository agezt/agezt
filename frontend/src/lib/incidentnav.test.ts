// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import {
  INCIDENT_HASH_PREFIX,
  incidentIdFromHash,
  openIncident,
} from "@/lib/incidentnav";

describe("incidentnav", () => {
  beforeEach(() => {
    location.hash = "";
  });

  it("incidentIdFromHash extracts the incident id from the route", () => {
    expect(incidentIdFromHash("#incident/root-1")).toBe("root-1");
    expect(incidentIdFromHash("#/incident/root-1")).toBe("root-1");
    expect(incidentIdFromHash("incident/root-1")).toBe("root-1");
  });

  it("returns null for normal views and blank incident selections", () => {
    expect(incidentIdFromHash("#autonomy")).toBeNull();
    expect(incidentIdFromHash("#incident/")).toBeNull();
    expect(incidentIdFromHash("")).toBeNull();
  });

  it("round-trips an id needing encoding", () => {
    const id = "builder/root-1";
    openIncident(id);
    expect(location.hash).toContain(INCIDENT_HASH_PREFIX);
    expect(incidentIdFromHash(location.hash)).toBe(id);
  });
});
