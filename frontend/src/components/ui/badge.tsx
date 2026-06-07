import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium",
  {
    variants: {
      variant: {
        default: "border-border bg-panel text-foreground",
        good: "border-good text-good",
        bad: "border-bad text-bad",
        warn: "border-warn text-warn",
        accent: "border-accent text-accent",
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
