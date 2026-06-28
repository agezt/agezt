import * as React from "react";
import { cn } from "@/lib/utils";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        "h-8 w-full rounded-md border border-border bg-panel px-2.5 text-sm outline-none transition-[border-color,box-shadow] placeholder:text-muted focus-glow",
        className,
      )}
      {...props}
    />
  ),
);
Input.displayName = "Input";

export const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.TextareaHTMLAttributes<HTMLTextAreaElement>
>(({ className, ...props }, ref) => (
  <textarea
    ref={ref}
    spellCheck={false}
    className={cn(
      "w-full resize-y rounded-md border border-border bg-panel p-2 font-mono text-xs outline-none transition-[border-color,box-shadow] placeholder:text-muted focus-visible:border-accent focus-visible:ring-2 focus-visible:ring-accent/30",
      className,
    )}
    {...props}
  />
));
Textarea.displayName = "Textarea";
