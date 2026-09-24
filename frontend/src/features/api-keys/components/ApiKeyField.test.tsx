// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { ApiKeyField } from "./ApiKeyField";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

afterEach(cleanup);

describe("ApiKeyField", () => {
  it("renders the env chip and a Save button", () => {
    render(withUI(<ApiKeyField env="OPENAI_API_KEY" value="" onChange={() => {}} onSubmit={() => {}} />));
    expect(screen.getByText("OPENAI_API_KEY")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Save" })).toBeTruthy();
  });

  it("masks the input by default and reveals on click", () => {
    const { container } = render(
      withUI(<ApiKeyField env="OPENAI_API_KEY" value="sk-1234" onChange={() => {}} onSubmit={() => {}} />),
    );
    const input = container.querySelector("input") as HTMLInputElement;
    expect(input.type).toBe("password");
    fireEvent.click(screen.getByRole("button", { name: "Reveal key" }));
    expect(input.type).toBe("text");
    fireEvent.click(screen.getByRole("button", { name: "Hide key" }));
    expect(input.type).toBe("password");
  });

  it("calls onSubmit with the trimmed value via the Save button", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(
      withUI(<ApiKeyField env="OPENAI_API_KEY" value="  sk-1234  " onChange={() => {}} onSubmit={onSubmit} />),
    );
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith("sk-1234"));
  });

  it("calls onSubmit with the trimmed value on Enter", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(
      withUI(<ApiKeyField env="OPENAI_API_KEY" value="sk-1234" onChange={() => {}} onSubmit={onSubmit} />),
    );
    const input = screen.getByLabelText("OPENAI_API_KEY API key");
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith("sk-1234"));
  });

  it("does NOT submit when value is empty or whitespace", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(withUI(<ApiKeyField env="OPENAI_API_KEY" value="   " onChange={() => {}} onSubmit={onSubmit} />));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSubmit).not.toHaveBeenCalled();
    const input = screen.getByLabelText("OPENAI_API_KEY API key");
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("shows the set badge and masked placeholder when isSet", () => {
    render(
      withUI(<ApiKeyField env="OPENAI_API_KEY" value="" isSet onChange={() => {}} onSubmit={() => {}} />),
    );
    expect(screen.getByText("set")).toBeTruthy();
    const input = screen.getByLabelText("OPENAI_API_KEY API key") as HTMLInputElement;
    expect(input.placeholder).toContain("set");
  });

  it("shows the pinned notice and hides the input when pinned", () => {
    render(
      withUI(
        <ApiKeyField env="OPENAI_API_KEY" value="" isSet pinned onChange={() => {}} onSubmit={() => {}} />,
      ),
    );
    expect(screen.getByText("Set from the environment.")).toBeTruthy();
    expect(screen.queryByLabelText("OPENAI_API_KEY API key")).toBeNull();
  });

  it("renders the Get-one link when provided", () => {
    render(
      withUI(
        <ApiKeyField
          env="OPENAI_API_KEY"
          value=""
          onChange={() => {}}
          onSubmit={() => {}}
          link="https://platform.openai.com/api-keys"
        />,
      ),
    );
    const link = screen.getByRole("link", { name: /Get one/ });
    expect(link.getAttribute("href")).toBe("https://platform.openai.com/api-keys");
  });

  it("shows the fingerprint chip when provided", () => {
    render(
      withUI(
        <ApiKeyField
          env="OPENAI_API_KEY"
          value=""
          fingerprint="…1111"
          isSet
          onChange={() => {}}
          onSubmit={() => {}}
        />,
      ),
    );
    expect(screen.getByText("…1111")).toBeTruthy();
  });
});
