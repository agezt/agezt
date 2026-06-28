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

import { AuthorSkillForm, Skills, skillMatches } from "@/views/Skills";
import { UIProvider } from "@/components/ui/feedback";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  postJSON.mockResolvedValue({ name: "deploy-release", status: "shadow" });
});

describe("AuthorSkillForm", () => {
  it("disables Create until name and body are provided", () => {
    render(<AuthorSkillForm onCreated={() => {}} onError={() => {}} />);
    const btn = () => screen.getByRole("button", { name: /Create skill/ }) as HTMLButtonElement;
    expect(btn().disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Skill name"), { target: { value: "deploy-release" } });
    expect(btn().disabled).toBe(true); // body still empty
    fireEvent.change(screen.getByLabelText("Skill body"), { target: { value: "do the steps" } });
    expect(btn().disabled).toBe(false);
  });

  it("posts name/body plus split trigger & tool lists, omitting empties", async () => {
    const onCreated = vi.fn();
    render(<AuthorSkillForm onCreated={onCreated} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Skill name"), { target: { value: "  deploy-release  " } });
    fireEvent.change(screen.getByLabelText("Skill body"), { target: { value: "  ship it  " } });
    fireEvent.change(screen.getByLabelText("Skill description"), { target: { value: "release flow" } });
    fireEvent.click(screen.getByRole("button", { name: /Matching and ownership/ }));
    fireEvent.change(screen.getByLabelText("Skill triggers"), { target: { value: " deploy , ship ,, release " } });
    fireEvent.change(screen.getByLabelText("Skill tools required"), { target: { value: "shell," } });
    fireEvent.click(screen.getByRole("button", { name: /Create skill/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/skill/import", {
        name: "deploy-release",
        body: "ship it",
        description: "release flow",
        triggers: ["deploy", "ship", "release"],
        tools_required: ["shell"],
      }),
    );
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith("deploy-release", "shadow"));
  });

  it("omits triggers/tools/description when blank", async () => {
    render(<AuthorSkillForm onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Skill name"), { target: { value: "tidy" } });
    fireEvent.change(screen.getByLabelText("Skill body"), { target: { value: "clean up" } });
    fireEvent.click(screen.getByRole("button", { name: /Create skill/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/skill/import", { name: "tidy", body: "clean up" }));
  });

  it("surfaces a create error", async () => {
    postJSON.mockRejectedValueOnce(new Error("empty body"));
    const onError = vi.fn();
    render(<AuthorSkillForm onCreated={() => {}} onError={onError} />);
    fireEvent.change(screen.getByLabelText("Skill name"), { target: { value: "x" } });
    fireEvent.change(screen.getByLabelText("Skill body"), { target: { value: "y" } });
    fireEvent.click(screen.getByRole("button", { name: /Create skill/ }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith("empty body"));
  });
});

describe("AuthorSkillForm (revise mode, M737)", () => {
  const initial = {
    name: "deploy-release",
    description: "release flow",
    body: "old steps",
    triggers: ["deploy", "ship"],
    tools_required: ["shell"],
  };

  it("prefills from the skill and labels the action Save as new version", () => {
    render(<AuthorSkillForm initial={initial} onCreated={() => {}} onError={() => {}} />);
    expect((screen.getByLabelText("Skill name") as HTMLInputElement).value).toBe("deploy-release");
    expect((screen.getByLabelText("Skill body") as HTMLTextAreaElement).value).toBe("old steps");
    expect((screen.getByLabelText("Skill triggers") as HTMLInputElement).value).toBe("deploy, ship");
    expect((screen.getByLabelText("Skill tools required") as HTMLInputElement).value).toBe("shell");
    expect(screen.queryByRole("button", { name: /Create skill/ })).toBeNull();
    expect(screen.getByRole("button", { name: /Save as new version/ })).toBeTruthy();
  });

  it("posts the revised body under the same name (a new version)", async () => {
    render(<AuthorSkillForm initial={initial} onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Skill body"), { target: { value: "new improved steps" } });
    fireEvent.click(screen.getByRole("button", { name: /Save as new version/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/skill/import", {
        name: "deploy-release",
        body: "new improved steps",
        description: "release flow",
        triggers: ["deploy", "ship"],
        tools_required: ["shell"],
      }),
    );
  });
});

describe("Skills filter (M778)", () => {
  it("authors a skill through a modal instead of an inline settings panel", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/skills/hygiene") return Promise.resolve({ idle: [] });
      return Promise.resolve({ skills: [], active: 0 });
    });
    render(
      <UIProvider>
        <Skills />
      </UIProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: /Author skill/ }));
    expect(screen.getByRole("dialog", { name: "Author skill" })).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Skill name"), { target: { value: "deploy-release" } });
    fireEvent.change(screen.getByLabelText("Skill body"), { target: { value: "ship it" } });
    fireEvent.click(screen.getByRole("button", { name: /Create skill/ }));

    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/skill/import", { name: "deploy-release", body: "ship it" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Author skill" })).toBeNull());
  });

  it("skillMatches matches on name, description, status, triggers, or tools", () => {
    const s = {
      name: "deploy-release",
      description: "ship a tagged build",
      status: "active",
      triggers: ["on tag push"],
      tools_required: ["shell", "http"],
    };
    expect(skillMatches(s, "deploy")).toBe(true);
    expect(skillMatches(s, "ship")).toBe(true);
    expect(skillMatches(s, "active")).toBe(true);
    expect(skillMatches(s, "tag push")).toBe(true);
    expect(skillMatches(s, "http")).toBe(true);
    expect(skillMatches(s, "nope")).toBe(false);
    expect(skillMatches(s, "")).toBe(true);
  });

  it("filters skill cards with a count once there are more than four", async () => {
    getJSON.mockResolvedValue({
      skills: [
        { id: "s1", name: "deploy-release", status: "active", description: "ship a build" },
        { id: "s2", name: "triage-bug", status: "shadow", description: "label issues" },
        { id: "s3", name: "summarize-thread", status: "active", description: "tl;dr a chat" },
        { id: "s4", name: "draft-standup", status: "draft", description: "morning note" },
        { id: "s5", name: "review-diff", status: "active", description: "read a PR" },
      ],
      active: 3,
    });
    render(
      <UIProvider>
        <Skills />
      </UIProvider>,
    );
    const input = await screen.findByLabelText("Filter skills");
    expect(screen.queryByText("1/5")).toBeNull();
    fireEvent.change(input, { target: { value: "shadow" } });
    await waitFor(() => expect(screen.getByText("1/5")).toBeTruthy());
    fireEvent.change(input, { target: { value: "zzz" } });
    await waitFor(() => expect(screen.getByText(/no skills match/)).toBeTruthy());
  });
});
