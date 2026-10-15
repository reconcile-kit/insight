import React from "react";
import { cn, hue } from "@/lib/utils";

/** Colored pill for a kind. The hue depends on group+kind so equal kind names in different groups differ. */
export function KindChip({ kind, group, className, ...props }) {
  return (
    <span
      className={cn("chip inline-flex h-5 items-center rounded-md px-1.5 text-[11px] font-semibold", className)}
      style={{ "--h": hue(`${group}/${kind}`) }}
      {...props}
    >
      {kind}
    </span>
  );
}

export function GroupLabel({ group, className }) {
  return (
    <span className={cn("inline-flex min-w-0 items-center gap-1.5", className)}>
      <span
        className="h-2 w-2 shrink-0 rounded-full"
        style={{ background: `hsl(${hue(group)} 70% 58%)`, boxShadow: `0 0 0 3px hsl(${hue(group)} 70% 58% / 0.18)` }}
      />
      <span className="truncate">{group}</span>
    </span>
  );
}

export function LabelChip({ k, v, onClick, title }) {
  const Comp = onClick ? "button" : "span";
  return (
    <Comp
      type={onClick ? "button" : undefined}
      onClick={onClick}
      title={title || `${k}=${v}`}
      className={cn(
        "inline-flex h-5 max-w-[220px] items-center overflow-hidden rounded-md border border-line bg-elevated font-mono text-[10.5px]",
        onClick && "transition-colors hover:border-accent/60 hover:bg-accent/10"
      )}
    >
      <span className="truncate px-1.5 text-muted">{k}</span>
      <span className="truncate border-l border-line px-1.5 text-fg">{v}</span>
    </Comp>
  );
}
