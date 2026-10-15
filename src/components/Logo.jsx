import React from "react";

/** The reconcile-kit mark: a reconcile loop with a check. Arrows follow the text color. */
export function Logo({ size = 28, className }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" fill="none" className={className} aria-label="reconcile-kit">
      <g stroke="currentColor" strokeWidth="2.6">
        <path d="M6.3 16.3A9.7 9.7 0 0 1 23.4 10.4" />
        <path d="M25.7 15.7A9.7 9.7 0 0 1 8.6 21.6" />
      </g>
      <g fill="currentColor">
        <path d="M26.2 8V13.5H20.7Z" />
        <path d="M5.8 24V18.5H11.3Z" />
      </g>
      <path
        d="M12.2 15.6 14.7 18.1 19.6 13.2"
        stroke="#22a447"
        strokeWidth="2.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
