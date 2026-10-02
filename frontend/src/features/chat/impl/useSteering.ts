import { useUI } from "@/components/ui/feedback";

interface UseSteeringParams {
  activeCorr: string | null;
  input: string;
  setInput: (value: string) => void;
  steer: (directive: string, mode: "steer" | "note") => Promise<void>;
}

export function useSteering({ activeCorr, input, setInput, steer }: UseSteeringParams) {
  const ui = useUI();

  // doSteer injects the composer text into the running run (M962): mode "steer"
  // re-prioritises, "note" is a soft BTW. Clears the box on success.
  async function doSteer(mode: "steer" | "note") {
    const t = input.trim();
    if (!t || !activeCorr) return;
    try {
      await steer(t, mode);
      setInput("");
      ui.toast(
        mode === "note"
          ? "BTW sent — the agent will read it and keep going"
          : "Steered — the agent will break at its next safe point",
        "success",
      );
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  return { doSteer };
}
