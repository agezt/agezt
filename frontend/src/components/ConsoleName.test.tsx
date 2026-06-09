// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { ConsoleName } from "@/components/ConsoleName";
import { saveConsoleName, DEFAULT_NAME } from "@/lib/brand";

beforeEach(() => {
  localStorage.clear();
  document.title = "";
  // Console name is a shared module store (so external setters stay in sync); reset
  // it between tests since the singleton carries state across them.
  saveConsoleName(DEFAULT_NAME);
  localStorage.clear();
});
afterEach(cleanup);

describe("ConsoleName", () => {
  it("shows the default name and renames inline on Enter (persists + sets title)", () => {
    render(<ConsoleName />);
    fireEvent.click(screen.getByRole("button", { name: /Rename console/ }));
    const input = screen.getByLabelText("Console name");
    fireEvent.change(input, { target: { value: "Jarvis" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(screen.getByRole("button", { name: /Rename console/ }).textContent).toBe("Jarvis");
    expect(localStorage.getItem("agezt-console-name")).toBe("Jarvis");
    expect(document.title).toBe("Jarvis · console");
  });

  it("cancels on Escape without renaming", () => {
    render(<ConsoleName />);
    fireEvent.click(screen.getByRole("button", { name: /Rename console/ }));
    const input = screen.getByLabelText("Console name");
    fireEvent.change(input, { target: { value: "Discarded" } });
    fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.getByRole("button", { name: /Rename console/ }).textContent).toBe("agezt");
    expect(localStorage.getItem("agezt-console-name")).toBeNull();
  });
});
