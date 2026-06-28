import * as React from "react";
import { cn } from "@/lib/utils";

export function Card({
  className,
  glass,
  interactive,
  ...props
}: React.HTMLAttributes<HTMLDivElement> & { glass?: boolean; interactive?: boolean }) {
  return (
    <div
      // Base: a lifted surface (shadow-e1, M951). `glass` (M978) swaps in the
      // command-center translucent treatment; `interactive` adds a hover lift for
      // clickable cards. Views still pass extra classes freely.
      className={cn(
        "flex min-h-0 flex-col overflow-hidden transition-[box-shadow,transform,border-color]",
        glass ? "glass rounded-xl" : "rounded-lg border border-border bg-card shadow-e1",
        interactive && "card-lift cursor-pointer",
        className,
      )}
      {...props}
    />
  );
}

export function CardHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("flex items-center gap-2 border-b border-border px-3 py-2", className)}
      {...props}
    />
  );
}

export function CardTitle({ className, ...props }: React.HTMLAttributes<HTMLHeadingElement>) {
  return (
    <h2
      className={cn("text-xs font-semibold uppercase tracking-normal text-muted", className)}
      {...props}
    />
  );
}

export function CardBody({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("min-h-0 overflow-auto p-3", className)} {...props} />;
}
