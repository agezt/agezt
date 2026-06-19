// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));

import { Channels } from "@/views/Channels";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

const CHANNELS = [
  {
    kind: "telegram",
    display: "Telegram",
    description: "Telegram bot",
    transport: "long-poll",
    duplex: true,
    configured: true,
    live: true,
    fields: [
      { env: "AGEZT_TELEGRAM_TOKEN", label: "Bot token", secret: true, required: true, set: true },
      { env: "AGEZT_TELEGRAM_CHAT_ID", label: "Allowed chats", set: false, value: "" },
    ],
  },
  {
    kind: "whatsapp",
    display: "WhatsApp",
    description: "WhatsApp Cloud API",
    transport: "webhook",
    duplex: true,
    configured: false,
    fields: [{ env: "AGEZT_WHATSAPP_ACCESS_TOKEN", label: "Access token", secret: true, required: true, set: false }],
  },
];

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  getJSON.mockResolvedValue({ channels: CHANNELS });
  postJSON.mockResolvedValue({});
});

const WGW_ONLY = [
  {
    kind: "whatsappgw",
    display: "WhatsApp (Gateway)",
    description: "WAHA/Evolution",
    transport: "rest",
    duplex: true,
    configured: true,
    live: false,
    fields: [
      { env: "AGEZT_WHATSAPPGW_URL", label: "Gateway URL", set: true, value: "http://localhost:3000" },
      { env: "AGEZT_WHATSAPPGW_BACKEND", label: "Backend", set: true, value: "waha" },
    ],
  },
];

describe("Channels", () => {
  it("probes the WhatsApp gateway connection", async () => {
    getJSON.mockResolvedValue({ channels: WGW_ONLY });
    postJSON.mockResolvedValue({ ok: true, connected: true, status: "WORKING" });
    render(withUI(<Channels />));
    await screen.findByText("WhatsApp (Gateway)");
    fireEvent.click(screen.getByRole("button", { name: /edit/i }));
    fireEvent.click(await screen.findByRole("button", { name: /check connection/i }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/whatsappgw/status", expect.objectContaining({
        url: "http://localhost:3000",
        backend: "waha",
      })),
    );
    expect(await screen.findByText(/logged in & ready/i)).toBeTruthy();
  });

  it("renders the gateway QR inline", async () => {
    getJSON.mockResolvedValue({ channels: WGW_ONLY });
    const png = "data:image/png;base64,iVBORw0KGgoAAAANS";
    postJSON.mockImplementation((path: string) =>
      path === "/api/whatsappgw/qr" ? Promise.resolve({ ok: true, qr: png }) : Promise.resolve({ ok: true, connected: false, status: "SCAN_QR_CODE" }),
    );
    render(withUI(<Channels />));
    await screen.findByText("WhatsApp (Gateway)");
    fireEvent.click(screen.getByRole("button", { name: /edit/i }));
    fireEvent.click(await screen.findByRole("button", { name: /show qr/i }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/whatsappgw/qr", expect.objectContaining({ url: "http://localhost:3000", backend: "waha" })),
    );
    const img = (await screen.findByAltText("WhatsApp login QR")) as HTMLImageElement;
    expect(img.src).toBe(png);
  });

  it("lists channels with connected / needs-setup state", async () => {
    render(withUI(<Channels />));
    expect(await screen.findByText("Telegram")).toBeTruthy();
    expect(screen.getByText("WhatsApp")).toBeTruthy();
    expect(screen.getByText("live")).toBeTruthy(); // telegram is running
    expect(screen.getByText("needs setup")).toBeTruthy(); // whatsapp not configured
    expect(screen.getByText(/2 channels · 1 live · 1 configured/)).toBeTruthy();
  });

  it("sends a test message via a live channel", async () => {
    render(withUI(<Channels />));
    await screen.findByText("Telegram");
    // Open Telegram (live) — its card button reads "Edit".
    fireEvent.click(screen.getByRole("button", { name: /edit/i }));
    fireEvent.change(await screen.findByLabelText("Test recipient"), { target: { value: "999" } });
    fireEvent.click(screen.getByRole("button", { name: /send test/i }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/send", {
        channel: "telegram",
        to: "999",
        text: "✅ AGEZT test message",
      }),
    );
  });

  it("saves an account field to the Config Center", async () => {
    render(withUI(<Channels />));
    await screen.findByText("WhatsApp");
    // Open WhatsApp's setup form (it's the "Connect" one).
    fireEvent.click(screen.getByRole("button", { name: /connect/i }));
    fireEvent.change(await screen.findByLabelText("Access token"), { target: { value: "EAAG-secret" } });
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", {
        name: "AGEZT_WHATSAPP_ACCESS_TOKEN",
        value: "EAAG-secret",
      }),
    );
  });
});
