import * as RadixTooltip from "@radix-ui/react-tooltip";

// Tooltip (M978) — a thin styled wrapper over @radix-ui/react-tooltip so the
// console can use real, accessible, themed tooltips (keyboard-focusable,
// collision-aware) instead of the browser's native title="" everywhere. Wrap the
// app once in <TooltipProvider>.
export const TooltipProvider = RadixTooltip.Provider;
