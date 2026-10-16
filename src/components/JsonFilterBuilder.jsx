import React, { useState } from "react";
import { Plus, X, Braces, HelpCircle } from "lucide-react";
import { cn } from "@/lib/utils";
import { Select } from "@/components/ui/select";
import { JSON_OPS, JSON_ROOTS, newJsonRow, opNeedsValue } from "@/lib/state";

/** Rows of `<root>.<path> <op> <value>` conditions over jsonb columns. */
export function JsonFilterBuilder({ rows, match, onChange, onMatchChange }) {
  const [help, setHelp] = useState(false);

  const update = (id, patch) => onChange(rows.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  const remove = (id) => onChange(rows.filter((r) => r.id !== id));

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="label mb-0 flex items-center gap-1.5">
          <Braces size={12} /> JSON conditions
          <button type="button" className="text-faint hover:text-fg" onClick={() => setHelp((h) => !h)} title="Path syntax">
            <HelpCircle size={12} />
          </button>
        </span>
        {rows.length > 1 && (
          <div className="flex rounded-md border border-line p-0.5 text-[10px] font-semibold uppercase">
            {[
              ["all", "AND"],
              ["any", "OR"],
            ].map(([m, l]) => (
              <button
                key={m}
                type="button"
                onClick={() => onMatchChange(m)}
                className={cn(
                  "rounded px-2 py-0.5 transition-colors",
                  match === m ? "bg-accent/20 text-accent" : "text-faint hover:text-fg"
                )}
              >
                {l}
              </button>
            ))}
          </div>
        )}
      </div>

      {help && (
        <div className="space-y-1 rounded-lg border border-line bg-surface p-2.5 font-mono text-[11px] leading-relaxed text-muted">
          <div><span className="text-fg">resources.cpu</span> · nested field</div>
          <div><span className="text-fg">conditions[0].type</span> · array element</div>
          <div><span className="text-fg">["app.kubernetes.io/name"]</span> · key with dots</div>
          <div className="pt-1 font-sans text-faint">Empty path — the whole {`spec/status/…`}.</div>
        </div>
      )}

      {rows.map((r) => (
        <div key={r.id} className="group relative space-y-1.5 rounded-xl border border-line bg-surface p-2">
          <div className="flex gap-1.5">
            <Select
              wrapperClassName="w-[124px]"
              className="h-8 font-mono text-[12px]"
              value={r.root}
              onChange={(e) => update(r.id, { root: e.target.value })}
            >
              {JSON_ROOTS.map((root) => (
                <option key={root}>{root}</option>
              ))}
            </Select>
            <input
              className="field h-8 min-w-0 flex-1 font-mono text-[12.5px]"
              value={r.path}
              spellCheck={false}
              placeholder="resources.cpu"
              onChange={(e) => update(r.id, { path: e.target.value })}
            />
          </div>
          <div className="flex gap-1.5">
            <Select
              wrapperClassName="w-[124px]"
              className="h-8 font-mono text-[12px]"
              value={r.op}
              title={JSON_OPS.find((o) => o.op === r.op)?.hint}
              onChange={(e) => update(r.id, { op: e.target.value })}
            >
              {JSON_OPS.map((o) => (
                <option key={o.op} value={o.op} title={o.hint}>
                  {o.label}
                </option>
              ))}
            </Select>
            {opNeedsValue(r.op) ? (
              <input
                className="field h-8 min-w-0 flex-1 font-mono text-[12.5px]"
                value={r.value}
                spellCheck={false}
                placeholder={r.op === "in" ? "a, b, c" : r.op === "matches" ? "^nginx" : "0"}
                onChange={(e) => update(r.id, { value: e.target.value })}
              />
            ) : (
              <div className="flex-1" />
            )}
          </div>
          <button
            type="button"
            onClick={() => remove(r.id)}
            className="absolute -right-2 -top-2 hidden h-5 w-5 items-center justify-center rounded-full border border-line bg-elevated text-faint hover:text-danger group-hover:flex"
            title="Remove condition"
          >
            <X size={11} />
          </button>
        </div>
      ))}

      <button
        type="button"
        onClick={() => onChange([...rows, newJsonRow()])}
        className="flex h-8 w-full items-center justify-center gap-1.5 rounded-xl border border-dashed border-line text-xs text-muted transition-colors hover:border-accent/50 hover:text-accent"
      >
        <Plus size={13} /> Condition
      </button>
    </div>
  );
}
