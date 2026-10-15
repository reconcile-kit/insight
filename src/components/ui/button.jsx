import React from "react";
import { cn } from "@/lib/utils";

const variants = {
  primary:
    "bg-accent text-accent-fg shadow-[0_1px_0_0_rgb(255_255_255/0.15)_inset,0_6px_20px_-8px_rgb(var(--accent)/0.7)] hover:brightness-110",
  secondary: "bg-elevated text-fg border border-line hover:border-faint/60 hover:bg-line/40",
  ghost: "text-muted hover:text-fg hover:bg-line/50",
  danger: "bg-danger/10 text-danger border border-danger/25 hover:bg-danger/20",
};

const sizes = {
  sm: "h-8 px-2.5 text-xs gap-1.5",
  md: "h-9 px-3.5 text-sm gap-2",
  icon: "h-8 w-8 justify-center",
};

export const Button = React.forwardRef(function Button(
  { className, variant = "secondary", size = "md", type = "button", ...props },
  ref
) {
  return (
    <button
      ref={ref}
      type={type}
      className={cn(
        "inline-flex select-none items-center whitespace-nowrap rounded-lg font-medium transition-all",
        "focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-accent/25 active:scale-[0.98]",
        "disabled:pointer-events-none disabled:opacity-40",
        variants[variant],
        sizes[size],
        className
      )}
      {...props}
    />
  );
});
