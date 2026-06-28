import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-md text-sm font-medium transition-[color,background-color,border-color,box-shadow,transform,filter] duration-150 outline-none focus-glow active:translate-y-px disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        // Subtle elevation on bordered/filled buttons makes them feel tactile;
        // ghost stays flat. Accent gets a brightness lift instead of an opacity
        // wash so its colour stays true on hover (M951).
        default: "border border-border bg-panel shadow-e1 hover:border-accent hover:bg-card",
        accent: "bg-accent text-white shadow-e1 hover:shadow-e2 hover:brightness-110 active:brightness-95",
        good: "border border-good text-good hover:bg-good hover:text-white",
        danger: "border border-bad text-bad hover:bg-bad hover:text-white",
        ghost: "hover:bg-panel hover:text-foreground",
      },
      size: {
        default: "h-8 px-3 py-1",
        sm: "h-7 px-2 text-xs",
        icon: "h-8 w-8",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => (
    <button ref={ref} className={cn(buttonVariants({ variant, size }), className)} {...props} />
  ),
);
Button.displayName = "Button";
