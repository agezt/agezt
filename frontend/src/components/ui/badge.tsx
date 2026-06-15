import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium",
  {
    variants: {
      // A soft tinted fill (colour/10) behind the coloured text reads as a richer,
      // more legible status chip than a hairline outline alone (M951). The text-*
      // colour classes are preserved for downstream tests/semantics.
      variant: {
        default: "border-border bg-panel text-foreground",
        good: "border-good/30 bg-good/10 text-good",
        bad: "border-bad/30 bg-bad/10 text-bad",
        warn: "border-warn/30 bg-warn/10 text-warn",
        accent: "border-accent/30 bg-accent/10 text-accent",
      },
    },
    defaultVariants: { variant: "default" },
  },
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />;
}

// statusBadge maps a run/plan status string to a coloured Badge.
export function statusVariant(status?: string): VariantProps<typeof badgeVariants>["variant"] {
  const s = (status || "").toLowerCase();
  if (s === "completed") return "good";
  if (s === "failed" || s === "abandoned") return "bad";
  if (s === "running") return "accent";
  return "default";
}
