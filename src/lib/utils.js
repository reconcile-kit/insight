import React from "react";
import { clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs) {
  return twMerge(clsx(inputs));
}

/** Stable hue for a string, so the same kind / group always gets the same color. */
export function hue(s) {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  // Golden-angle spread keeps neighbouring hashes visually apart.
  return Math.round(((h >>> 0) * 137.508) % 360);
}

export function timeAgo(ts) {
  if (!ts) return "—";
  const s = Math.max(0, Math.floor((Date.now() - new Date(ts).getTime()) / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h`;
  const d = Math.floor(h / 24);
  if (d < 365) return `${d}d`;
  return `${Math.floor(d / 365)}y`;
}

export function fmtDate(ts) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("ru-RU", { dateStyle: "medium", timeStyle: "medium" });
}

export const fmtNum = (n) => (n ?? 0).toLocaleString("ru-RU");

export function useDebounced(value, ms) {
  const [v, setV] = React.useState(value);
  React.useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}
