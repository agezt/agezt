import * as React from "react";
import { cn } from "@/app/utils";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        "h-8 w-full rounded-md border border-border bg-panel px-2.5 text-sm outline-none transition-[border-color,box-shadow] placeholder:text-muted hover:border-accent/40 focus-glow",
        className,
      )}
      {...props}
    />
  ),
);
Input.displayName = "Input";

