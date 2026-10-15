import React, { useRef } from "react";
import { Plus, X, Braces } from "lucide-react";
import { Suggest } from "@/components/Suggest";
import { cn } from "@/lib/utils";
import { fetchPaths } from "@/lib/api";
import { JSON_OPS, JSON_ROOTS, newJsonRow, opNeedsValue } from "@/lib/state";

const typeHint = { number: "num", string: "str", boolean: "bool", object: "obj", array: "arr", null: "null" };

/** Rows of `<root>.<path> <op> <value>` conditions over jsonb columns. */
export function JsonFilterBuilder({ rows, match, onChange, onMatchChange, baseState }) {
  // Paths are cached per root and per top-level filters (they narrow the sample).
  const cache = useRef(new Map());
  const baseKey = JSON.stringify([
    baseState.resource_group,
    baseState.kind,
    baseState.namespace,
    baseState.shard_id,
  ]);

  const loadPaths = async (root, q, signal) => {
    const key = root + baseKey;
    let paths = cache.current.get(key);
    if (!paths) {
      paths = await fetchPaths(root, { ...baseState, json: [], labels: "", name: "" }, signal);
      cache.current.set(key, paths);
    }
    const needle = q.toLowerCase();
    return paths
      .filter((p) => !needle || p.path.toLowerCase().includes(needle))
      .slice(0, 200)
      .map((p) => ({ value: p.path, hint: `${typeHint[p.type] || p.type} · ${p.count}` }));
  };

  const update = (id, patch) => onChange(rows.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  const remove = (id) => onChange(rows.filter((r) => r.id !== id));

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="label mb-0 flex items-center gap-1.5">
          <Braces size={12} /> JSON-условия
        </span>
        {rows.length > 1 && (
          <div className="flex rounded-md border border-line p-0.5 text-[10px] font-semibold uppercase">
            {[
              ["all", "И"],
              ["any", "ИЛИ"],
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

      {rows.map((r) => (
        <div key={r.id} className="group relative space-y-1.5 rounded-xl border border-line bg-surface p-2">
          <div className="flex gap-1.5">
            <select
              className="field h-8 w-[92px] shrink-0 px-2 font-mono text-[12px]"
              value={r.root}
              onChange={(e) => update(r.id, { root: e.target.value })}
            >
              {JSON_ROOTS.map((root) => (
                <option key={root}>{root}</option>
              ))}
            </select>
            <Suggest
              className="min-w-0 flex-1"
              inputClassName="h-8"
              mono
              value={r.path}
              placeholder="exit_code"
              onChange={(path) => update(r.id, { path })}
              onPick={(it) => update(r.id, { path: it.value })}
              load={(q, signal) => loadPaths(r.root, q, signal)}
              emptyText="Путей не найдено"
              menuClassName="left-auto w-[262px]"
            />
          </div>
          <div className="flex gap-1.5">
            <select
              className="field h-8 w-[92px] shrink-0 px-2 font-mono text-[12px]"
              value={r.op}
              title={JSON_OPS.find((o) => o.op === r.op)?.hint}
              onChange={(e) => update(r.id, { op: e.target.value })}
            >
              {JSON_OPS.map((o) => (
                <option key={o.op} value={o.op} title={o.hint}>
                  {o.label}
                </option>
              ))}
            </select>
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
            title="Удалить условие"
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
        <Plus size={13} /> Условие
      </button>
    </div>
  );
}
