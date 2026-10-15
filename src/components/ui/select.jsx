import React from "react";
import { ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

/** Native select without the browser arrow: our chevron keeps a proper inset from the edge. */
export function Select({ className, wrapperClassName, children, ...props }) {
  return (
    <div className={cn("relative shrink-0", wrapperClassName)}>
      <select className={cn("field cursor-pointer appearance-none pl-2.5 pr-7", className)} {...props}>
        {children}
      </select>
      <ChevronDown
        size={13}
        className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-faint"
      />
    </div>
  );
}
