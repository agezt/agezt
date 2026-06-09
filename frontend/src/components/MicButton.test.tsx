// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { MicButton } from "@/components/MicButton";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

afterEach(cleanup);

describe("MicButton", () => {
  it("renders the record affordance", () => {
    render(withUI(<MicButton onText={() => {}} />));
    expect(screen.getByTitle("Record a voice message")).toBeTruthy();
  });

  it("degrades gracefully with a toast when recording isn't supported", async () => {
    // jsdom has no MediaRecorder / mediaDevices → the unsupported path.
    render(withUI(<MicButton onText={() => {}} />));
    fireEvent.click(screen.getByTitle("Record a voice message"));
    await waitFor(() =>
      expect(screen.getByText(/isn't supported|access the microphone/i)).toBeTruthy(),
    );
  });
});
