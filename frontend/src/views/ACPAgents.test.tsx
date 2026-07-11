// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...args: unknown[]) => getJSON(...args),
}));

import { ACPAgents } from "@/views/ACPAgents";
import { UIProvider } from "@/components/ui/feedback";

const baseInventory = {
  os: "windows",
  active_command: "",
  installed_count: 1,
  missing_count: 1,
  agents: [
    {
      slug: "gemini", name: "Gemini CLI", bin: "gemini", command: "gemini --acp",
      description: "Gemini native ACP agent", installed: true, version: "0.50.0",
      path: "C:\\tools\\gemini.exe", active: true, install: "",
    },
    {
      slug: "codex-acp", name: "Codex", bin: "", command: "",
      description: "Codex through ACP", installed: false, version: "",
      path: "", active: false, install: "npx --yes @agentclientprotocol/codex-acp",
    },
  ],
};

beforeEach(() => {
  getJSON.mockReset().mockResolvedValue(baseInventory);
});
afterEach(cleanup);

describe("ACPAgents", () => {
  it("renders the page header and stats from the inventory", async () => {
    render(<UIProvider><ACPAgents /></UIProvider>);
    expect(await screen.findByText("ACP Agents")).toBeTruthy();
    expect(screen.getByText(/external coding agents that speak/i)).toBeTruthy();
    // Three stat cards: catalog=2, installed=1, missing=1
    expect(screen.getByText("2")).toBeTruthy();
    expect(screen.getAllByText("1").length).toBe(2);
    expect(getJSON).toHaveBeenCalledWith("/api/acp/agents");
  });

  it("shows each agent with correct name, status badge, and metadata", async () => {
    render(<UIProvider><ACPAgents /></UIProvider>);
    await screen.findByText("Gemini CLI");
    expect(screen.getByText("Codex")).toBeTruthy();
    expect(screen.getByText("gemini")).toBeTruthy();
    expect(screen.getByText("codex-acp")).toBeTruthy();

    // Installed badge vs missing badge
    expect(screen.getAllByText("installed").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("missing").length).toBeGreaterThanOrEqual(1);

    // Installed shows version
    expect(screen.getByText("0.50.0")).toBeTruthy();

    // Active agent gets a star badge
    expect(screen.getByTitle("Configured default ACP agent")).toBeTruthy();
  });

  it("shows copyable launch command for installed agents", async () => {
    const writeText = vi.fn(() => Promise.resolve());
    Object.assign(navigator, { clipboard: { writeText } });
    render(<UIProvider><ACPAgents /></UIProvider>);
    const btn = await screen.findByTitle("Copy launch command");
    expect(btn.textContent).toContain("gemini --acp");
  });

  it("shows copyable install hint for missing agents", async () => {
    const writeText = vi.fn(() => Promise.resolve());
    Object.assign(navigator, { clipboard: { writeText } });
    render(<UIProvider><ACPAgents /></UIProvider>);
    const btn = await screen.findByTitle("Copy install command");
    expect(btn.textContent).toContain("npx --yes @agentclientprotocol/codex-acp");
  });

  it("shows usage hints per agent", async () => {
    render(<UIProvider><ACPAgents /></UIProvider>);
    await screen.findByText("Gemini CLI");
    expect(screen.getByText(/default acp agent/i)).toBeTruthy();
    expect(screen.getAllByText(/install:/i).length).toBeGreaterThanOrEqual(1);  // install hint for codex
  });

  it("shows the host & usage disclosure with OS info", async () => {
    render(<UIProvider><ACPAgents /></UIProvider>);
    await screen.findByText("ACP Agents");
    expect(screen.getByText(/no default agent configured/i)).toBeTruthy();
    expect(screen.getByText("windows")).toBeTruthy();
  });

  it("re-fetches on refresh button click", async () => {
    render(<UIProvider><ACPAgents /></UIProvider>);
    await screen.findByText("Gemini CLI");
    getJSON.mockClear();
    fireEvent.click(screen.getByTitle("Re-scan host"));
    await waitFor(() => expect(getJSON).toHaveBeenCalledTimes(1));
  });

  it("shows an error state when the API call fails", async () => {
    getJSON.mockRejectedValue(new Error("connection refused"));
    render(<UIProvider><ACPAgents /></UIProvider>);
    expect(await screen.findByText("connection refused")).toBeTruthy();
  });

  it("shows an empty state when the catalog has no agents", async () => {
    getJSON.mockResolvedValue({ os: "linux", active_command: "gemini", agents: [], installed_count: 0, missing_count: 0 });
    render(<UIProvider><ACPAgents /></UIProvider>);
    expect(await screen.findByText(/no acp agents in the catalog/i)).toBeTruthy();
  });
});
