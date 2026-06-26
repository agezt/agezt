// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));

import { JarvisPresenceCard } from "@/components/JarvisPresenceCard";

afterEach(cleanup);
beforeEach(() => getJSON.mockReset());

describe("JarvisPresenceCard", () => {
  it("links to the Jarvis page and reflects the live initiative mode", async () => {
    getJSON.mockResolvedValue({ initiative: "act", running: true, paused: false });
    const { container } = render(<JarvisPresenceCard />);

    // Triad labels are always present; the act-mode line comes from /api/pulse.
    expect(screen.getByText(/hears you/)).toBeTruthy();
    expect(screen.getByText(/knows you/)).toBeTruthy();
    await waitFor(() => expect(screen.getByText(/acting on its own/)).toBeTruthy());

    const link = container.querySelector('a[href="#jarvis"]');
    expect(link).toBeTruthy();
  });

  it("degrades gracefully when /api/pulse returns nothing", async () => {
    getJSON.mockResolvedValue(null);
    const { container } = render(<JarvisPresenceCard />);
    // No live data → the card still renders the triad + link, no crash.
    expect(screen.getByText(/hears you/)).toBeTruthy();
    expect(screen.getByText(/knows you/)).toBeTruthy();
    await waitFor(() => expect(container.querySelector('a[href="#jarvis"]')).toBeTruthy());
  });
});
