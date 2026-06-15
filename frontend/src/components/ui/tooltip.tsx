import * as React from "react";
import * as RadixTooltip from "@radix-ui/react-tooltip";
import { cn } from "@/lib/utils";

// Tooltip (M978) — a thin styled wrapper over @radix-ui/react-tooltip so the
// console can use real, accessible, themed tooltips (keyboard-focusable,
// collision-aware) instead of the browser's native title="" everywhere. Wrap the
// app once in <TooltipProvider>; use <Tip text="…"><button/></Tip> at call sites.
export const TooltipProvider = RadixTooltip.Provider;

export function Tip({
  text,
  children,
  side = "top",
  delayDuration = 200,
}: {
  text: React.ReactNode;
  children: React.ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  delayDuration?: number;
}) {
  if (!text) return <>{children}</>;
  return (
    <RadixTooltip.Root delayDuration={delayDuration}>
      <RadixTooltip.Trigger asChild>{children}</RadixTooltip.Trigger>
      <RadixTooltip.Portal>
        <RadixTooltip.Content
          side={side}
          sideOffset={6}
          className={cn(
            "z-[200] max-w-xs rounded-md border border-border bg-card px-2 py-1 text-xs text-foreground shadow-e2",
            "data-[state=delayed-open]:animate-in data-[state=closed]:animate-out",
          )}
        >
          {text}
          <RadixTooltip.Arrow className="fill-card" />
        </RadixTooltip.Content>
      </RadixTooltip.Portal>
    </RadixTooltip.Root>
  );
}
