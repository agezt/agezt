import { SlidersHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useAdvanced } from "@/lib/advanced";

// AdvancedToggle flips global Advanced mode. Calm by default; when on it lights
// up (accent) and the console reveals its text-heavy / diagnostic detail.
export function AdvancedToggle() {
  const { advanced, toggle } = useAdvanced();
  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={toggle}
      title={advanced ? "Advanced mode: on — showing diagnostics & detail" : "Advanced mode: off — calm view"}
      aria-label="Toggle advanced mode"
      aria-pressed={advanced}
      className={cn(advanced && "text-accent")}
    >
      <SlidersHorizontal />
    </Button>
  );
}
