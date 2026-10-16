import React, { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";

/**
 * Text input with a suggestion list.
 *
 * load(q, signal) returns [{ value, hint? }]. Enter picks the highlighted item;
 * with nothing highlighted Enter is left to the surrounding form (= apply filters).
 * While the user types, the first suggestion is highlighted automatically.
 */
export function Suggest({
  value,
  onChange,
  onPick,
  load,
  placeholder,
  className,
  inputClassName,
  mono,
  icon,
  right,
  emptyText = "No matches",
  autoFocus,
}) {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);
  const [hl, setHl] = useState(-1);
  const [typed, setTyped] = useState(false);
  const listRef = useRef(null);
  const loadRef = useRef(load);
  loadRef.current = load;

  const q = typed ? value : "";

  useEffect(() => {
    if (!open) return;
    const ctl = new AbortController();
    setLoading(true);
    const t = setTimeout(async () => {
      try {
        const res = await loadRef.current(q, ctl.signal);
        setItems(res);
        setHl(typed && q && res.length ? 0 : -1);
      } catch (e) {
        if (e.name !== "AbortError") setItems([]);
      } finally {
        if (!ctl.signal.aborted) setLoading(false);
      }
    }, 120);
    return () => {
      clearTimeout(t);
      ctl.abort();
    };
  }, [open, q, typed]);

  useEffect(() => {
    if (hl < 0 || !listRef.current) return;
    listRef.current.children[hl]?.scrollIntoView({ block: "nearest" });
  }, [hl]);

  const pick = (item) => {
    onPick(item);
    setOpen(false);
    setTyped(false);
  };

  const onKeyDown = (e) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setOpen(true);
      setHl((h) => Math.min(items.length - 1, h + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setHl((h) => Math.max(-1, h - 1));
    } else if (e.key === "Enter") {
      if (open && hl >= 0 && items[hl]) {
        e.preventDefault();
        pick(items[hl]);
      } else {
        setOpen(false);
      }
    } else if (e.key === "Escape") {
      if (open) {
        e.stopPropagation();
        setOpen(false);
      }
    } else if (e.key === "Tab") {
      setOpen(false);
    }
  };

  return (
    <div className={cn("relative", className)}>
      {icon && <span className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-faint">{icon}</span>}
      <input
        className={cn("field", icon && "pl-8", right && "pr-8", mono && "font-mono text-[12.5px]", inputClassName)}
        value={value}
        placeholder={placeholder}
        autoFocus={autoFocus}
        spellCheck={false}
        autoComplete="off"
        onChange={(e) => {
          onChange(e.target.value);
          setTyped(true);
          setOpen(true);
        }}
        onFocus={() => {
          setTyped(false);
          setOpen(true);
        }}
        onBlur={() => setTimeout(() => setOpen(false), 120)}
        onKeyDown={onKeyDown}
      />
      {right && <span className="absolute right-1 top-1/2 -translate-y-1/2">{right}</span>}
      {open && (
        <div className="absolute left-0 right-0 top-[calc(100%+6px)] z-30"><div className="overflow-hidden rounded-xl border border-line bg-elevated shadow-float animate-in fade-in-0 zoom-in-95 slide-in-from-top-1 duration-100">
          {loading && items.length === 0 ? (
            <div className="px-3 py-2.5 text-xs text-faint">Loading…</div>
          ) : items.length === 0 ? (
            <div className="px-3 py-2.5 text-xs text-faint">{emptyText}</div>
          ) : (
            <ul ref={listRef} className="max-h-72 overflow-auto p-1">
              {items.map((it, i) => (
                <li
                  key={it.key ?? it.value}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    pick(it);
                  }}
                  onMouseEnter={() => setHl(i)}
                  className={cn(
                    "flex cursor-pointer items-center justify-between gap-3 rounded-lg px-2.5 py-1.5 text-[13px]",
                    i === hl ? "bg-accent/15 text-fg" : "text-muted",
                    it.value === value && "font-semibold text-fg",
                    it.selected && "ring-1 ring-inset ring-accent/40"
                  )}
                >
                  <span className={cn("min-w-0 truncate", mono && "font-mono text-[12px]")}>{it.label ?? it.value}</span>
                  {it.hint && <span className="shrink-0 font-mono text-[10px] text-faint">{it.hint}</span>}
                </li>
              ))}
            </ul>
          )}
        </div></div>
      )}
    </div>
  );
}
