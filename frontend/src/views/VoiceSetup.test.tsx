// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";

interface TestProvider {
  id: string;
  name: string;
  blurb: string;
  baseURL: string;
  needsKey: boolean;
  dialect?: string;
  models: { id: string; label?: string; note?: string }[];
  voices?: { id: string; label?: string }[];
  voicesByModel?: Record<string, { id: string; label?: string }[]>;
  voiceFree?: boolean;
  voiceInModel?: boolean;
  keyHint?: string;
  keyLink?: string;
}

const { getJSON, postJSON, toast, stt, tts } = vi.hoisted(() => ({
  getJSON: vi.fn(),
  postJSON: vi.fn(),
  toast: vi.fn(),
  stt: [{
    id: "stt",
    name: "Test STT",
    blurb: "Test listener",
    baseURL: "https://stt.test/v1",
    needsKey: false,
    models: [{ id: "hear", label: "Hear", note: "accurate" }],
  }] as TestProvider[],
  tts: [{
    id: "tts",
    name: "Test TTS",
    blurb: "Test speaker",
    baseURL: "https://tts.test/v1",
    needsKey: true,
    models: [{ id: "speak", label: "Speak" }, { id: "sing" }],
    voices: [{ id: "one", label: "Voice One" }, { id: "two" }],
  }] as TestProvider[],
}));

vi.mock("@/lib/api", () => ({
  getJSON: (...args: unknown[]) => getJSON(...args),
  postJSON: (...args: unknown[]) => postJSON(...args),
}));
vi.mock("@/components/ui/feedback", () => ({ useUI: () => ({ toast }) }));
vi.mock("@/views/ConfigCenter", () => ({
  FieldRow: ({ field, onSaved, toast: notify }: {
    field: { env: string };
    onSaved: () => Promise<void>;
    toast: (text: string, kind?: "success") => void;
  }) => (
    <button type="button" onClick={() => { notify(field.env, "success"); void onSaved(); }}>
      edit {field.env}
    </button>
  ),
}));
vi.mock("@/lib/voiceCatalog", () => ({
  STT_PROVIDERS: stt,
  TTS_PROVIDERS: tts,
  dialectOf: (provider: { dialect?: string }) => provider.dialect || "openai",
  selectProvider: (providers: { baseURL: string; dialect?: string }[], url?: string, dialect?: string) => {
    if (dialect && dialect !== "openai") return providers.find((provider) => provider.dialect === dialect);
    return providers.find((provider) => provider.baseURL === url);
  },
  voicesFor: (provider?: { voices?: { id: string; label?: string }[]; voicesByModel?: Record<string, { id: string }[]> }, model?: string) =>
    (model && provider?.voicesByModel?.[model]) || provider?.voices || [],
}));

import { VoiceSetup } from "@/views/VoiceSetup";

const values = (fields: { env: string; value?: string; set?: boolean; env_pinned?: boolean }[]) => ({ fields });

function config(overrides: Record<string, string | undefined> = {}) {
  const base: Record<string, string | undefined> = {
    AGEZT_STT_PROVIDER: "openai",
    AGEZT_STT_URL: "https://stt.test/v1",
    AGEZT_STT_MODEL: "hear",
    AGEZT_TTS_PROVIDER: "openai",
    AGEZT_TTS_URL: "https://tts.test/v1",
    AGEZT_TTS_MODEL: "speak",
    AGEZT_TTS_VOICE: "one",
    ...overrides,
  };
  return values(Object.entries(base).map(([env, value]) => ({ env, value, set: value !== undefined && value !== "" })));
}

function route(current: ReturnType<typeof config>) {
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/config/values") return Promise.resolve(current);
    return Promise.resolve({
      sections: [{
        id: "voice",
        fields: Object.keys(current.fields.reduce<Record<string, true>>((all, field) => ({ ...all, [field.env]: true }), {}))
          .map((env) => ({ env, label: env, type: "text" })),
      }],
    });
  });
}

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  toast.mockReset();
  postJSON.mockResolvedValue({});
  stt[0].models = [{ id: "hear", label: "Hear", note: "accurate" }];
  tts[0].models = [{ id: "speak", label: "Speak" }, { id: "sing" }];
  tts[0].voices = [{ id: "one", label: "Voice One" }, { id: "two" }];
  delete tts[0].voicesByModel;
  delete tts[0].voiceFree;
  delete tts[0].voiceInModel;
});

describe("VoiceSetup branch behavior", () => {
  it("selects a provider and repairs an unknown model and voice", async () => {
    route(config({ AGEZT_TTS_URL: "https://custom.test/v1", AGEZT_TTS_MODEL: "old", AGEZT_TTS_VOICE: "old" }));
    render(<VoiceSetup />);
    fireEvent.click(await screen.findByRole("button", { name: "Test TTS" }));
    await waitFor(() => {
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_URL", value: "https://tts.test/v1" });
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_MODEL", value: "speak" });
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_VOICE", value: "one" });
    });
    expect(toast).toHaveBeenCalledWith(expect.stringContaining("selected"), "success");
  });

  it.each([
    ["provider", 0],
    ["url", 1],
    ["model", 2],
    ["voice", 3],
  ])("stops provider selection when saving %s fails", async (_name, successfulWrites) => {
    route(config({ AGEZT_TTS_URL: "https://custom.test/v1", AGEZT_TTS_MODEL: "old", AGEZT_TTS_VOICE: "old" }));
    for (let index = 0; index < successfulWrites; index += 1) postJSON.mockResolvedValueOnce({});
    postJSON.mockRejectedValueOnce(new Error("blocked"));
    render(<VoiceSetup />);
    fireEvent.click(await screen.findByRole("button", { name: "Test TTS" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("blocked", "error"));
    expect(postJSON).toHaveBeenCalledTimes(successfulWrites + 1);
  });

  it("keeps a compatible model and voice while changing provider", async () => {
    route(config({ AGEZT_TTS_URL: "https://custom.test/v1" }));
    render(<VoiceSetup />);
    fireEvent.click(await screen.findByRole("button", { name: "Test TTS" }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledTimes(2));
  });

  it("repairs a model-dependent voice and handles both save failures", async () => {
    tts[0].voicesByModel = { speak: [{ id: "one" }], sing: [{ id: "singer" }] };
    route(config({ AGEZT_TTS_VOICE: "old" }));
    render(<VoiceSetup />);
    const card = (await screen.findByRole("heading", { name: "Voice", level: 4 })).closest(".flex.flex-col") as HTMLElement;
    postJSON.mockRejectedValueOnce(new Error("model blocked"));
    fireEvent.click(within(card).getByRole("button", { name: "sing" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("model blocked", "error"));

    postJSON.mockResolvedValueOnce({}).mockRejectedValueOnce(new Error("voice blocked"));
    fireEvent.click(within(card).getByRole("button", { name: "sing" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("voice blocked", "error"));

    postJSON.mockResolvedValue({});
    fireEvent.click(within(card).getByRole("button", { name: "sing" }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_VOICE", value: "singer" }));
    expect(toast).toHaveBeenCalledWith("Saved — restart to apply", "success");
  });

  it("renders fallback values for empty models, labels, notes, hints, and voice lists", async () => {
    tts[0].models = [];
    tts[0].voices = [];
    route(config({ AGEZT_TTS_MODEL: "unknown", AGEZT_TTS_VOICE: "old" }));
    const { unmount } = render(<VoiceSetup />);
    expect(await screen.findByPlaceholderText("voice name (depends on your server)")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Test TTS" }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_URL", value: "https://tts.test/v1" }));
    fireEvent.change(screen.getByPlaceholderText("voice name (depends on your server)"), { target: { value: "new" } });
    fireEvent.blur(screen.getByPlaceholderText("voice name (depends on your server)"));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_VOICE", value: "new" }));
    unmount();

    tts[0].models = [{ id: "plain" }];
    tts[0].voiceFree = true;
    route(config({ AGEZT_TTS_MODEL: "unknown", AGEZT_TTS_VOICE: undefined, AGEZT_TTS_KEY: "secret" }));
    render(<VoiceSetup />);
    expect(await screen.findByRole("button", { name: "plain" })).toBeTruthy();
    expect(screen.getByPlaceholderText("voice id")).toBeTruthy();
    expect(screen.getByText("1/2 ready")).toBeTruthy();
  });

  it("changes an STT model without trying to save a voice", async () => {
    stt[0].models = [{ id: "hear", label: "Hear" }, { id: "listen" }];
    route(config());
    render(<VoiceSetup />);
    fireEvent.click(await screen.findByRole("button", { name: "listen" }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_STT_MODEL", value: "listen" }),
    );
    expect(toast).toHaveBeenCalledWith("Saved — restart to apply", "success");
  });

  it("renders a pinned key and exercises advanced field callbacks", async () => {
    const current = config();
    current.fields.push({ env: "AGEZT_TTS_KEY", value: "secret", set: true, env_pinned: true });
    route(current);
    render(<VoiceSetup />);
    expect(await screen.findByText("Set from the environment.")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: /Advanced · custom endpoint/ })[1]);
    fireEvent.click(await screen.findByRole("button", { name: "edit AGEZT_TTS_URL" }));
    expect(toast).toHaveBeenCalledWith("AGEZT_TTS_URL", "success");
    await waitFor(() => expect(getJSON).toHaveBeenCalledWith("/api/config/values"));
  });

  it("handles a key Enter without a draft and a key save without a link or hint", async () => {
    delete tts[0].keyHint;
    delete tts[0].keyLink;
    route(config());
    render(<VoiceSetup />);
    const key = await screen.findByPlaceholderText("paste your key");
    fireEvent.keyDown(key, { key: "Enter" });
    expect(postJSON).not.toHaveBeenCalled();
    fireEvent.change(key, { target: { value: " secret " } });
    fireEvent.click(screen.getAllByRole("button", { name: "Save" })[0]);
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/config/set", { name: "AGEZT_TTS_KEY", value: "secret" }));
  });
});
