// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));

import { Connections } from "@/views/Connections";

beforeEach(() => {
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/catalog") return Promise.resolve({ providers: [{ id: "deepseek", name: "DeepSeek", credentialed: true }, { id: "openai", credentialed: false }] });
    if (path === "/api/channels") return Promise.resolve({ channels: [{ kind: "telegram", display: "Telegram", live: true, configured: true }, { kind: "irc", display: "IRC", live: false, configured: true }] });
    if (path === "/api/mcp") return Promise.resolve({ servers: [{ name: "fetch", attached: true, enabled: true }] });
    return Promise.resolve({});
  });
});
afterEach(cleanup);

describe("Connections", () => {
  it("summarizes connected providers, channels and MCP servers", async () => {
    render(<Connections />);
    expect(await screen.findByText("Connections")).toBeTruthy();
    expect(await screen.findByText("DeepSeek")).toBeTruthy(); // keyed provider listed
    expect(screen.getByText("Telegram")).toBeTruthy(); // live channel
    expect(screen.getByText(/restart to start/i)).toBeTruthy(); // configured-not-live
    expect(screen.getByText("fetch")).toBeTruthy(); // attached MCP
  });

  it("navigates to the manage views via hash", async () => {
    render(<Connections />);
    await screen.findByText("Connections");
    fireEvent.click(screen.getByRole("button", { name: /connect a provider/i }));
    expect(location.hash).toBe("#quickconnect");
    fireEvent.click(screen.getByRole("button", { name: /manage channels/i }));
    expect(location.hash).toBe("#channels");
  });
});
