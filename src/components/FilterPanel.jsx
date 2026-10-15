import React, { useState } from "react";
import { X, Tags, HelpCircle, RotateCcw, CornerDownLeft } from "lucide-react";
import { Suggest } from "@/components/Suggest";
import { JsonFilterBuilder } from "@/components/JsonFilterBuilder";
import { Button } from "@/components/ui/button";
import { fetchValues } from "@/lib/api";
import { cn } from "@/lib/utils";

const FIELDS = [
  ["resource_group", "Group"],
  ["kind", "Kind"],
  ["namespace", "Namespace"],
  ["shard_id", "Shard"],
];

function Toggle({ checked, onChange, children, tone }) {
  return (
    <button
      type="button"
      onClick={() => onChange(!checked)}
      className={cn(
        "flex h-8 flex-1 items-center justify-center gap-1.5 rounded-lg border text-xs font-medium transition-colors",
        checked
          ? tone === "danger"
            ? "border-danger/40 bg-danger/10 text-danger"
            : "border-warn/40 bg-warn/10 text-warn"
          : "border-line text-muted hover:text-fg"
      )}
    >
      <span className={cn("h-1.5 w-1.5 rounded-full", checked ? "bg-current" : "bg-faint/50")} />
      {children}
    </button>
  );
}

/**
 * Sidebar with all filters. It edits the draft; Enter in any field (or the
 * Apply button) applies it. Picking a value from a dropdown applies at once.
 */
export function FilterPanel({ draft, setDraft, onApply, onReset }) {
  const [help, setHelp] = useState(false);
  const set = (patch) => setDraft((d) => ({ ...d, ...patch }));

  return (
    <form
      className="flex h-full flex-col"
      onSubmit={(e) => {
        e.preventDefault();
        onApply();
      }}
    >
      <div className="flex-1 space-y-5 overflow-y-auto px-4 pb-4 pt-4">
        {FIELDS.map(([field, label]) => (
          <div key={field}>
            <label className="label">{label}</label>
            <Suggest
              value={draft[field]}
              placeholder={`Любой ${label.toLowerCase()}`}
              onChange={(v) => set({ [field]: v })}
              onPick={(it) => onApply({ ...draft, [field]: it.value })}
              load={(q, signal) =>
                fetchValues(field, q, draft, signal).then((vals) => vals.map((v) => ({ value: v })))
              }
              right={
                draft[field] && (
                  <button
                    type="button"
                    className="flex h-7 w-7 items-center justify-center rounded-md text-faint hover:text-fg"
                    onClick={() => onApply({ ...draft, [field]: "" })}
                    title="Сбросить"
                  >
                    <X size={13} />
                  </button>
                )
              }
            />
          </div>
        ))}

        <div>
          <div className="flex items-center justify-between">
            <label className="label flex items-center gap-1.5">
              <Tags size={12} /> Labels
            </label>
            <button
              type="button"
              className="mb-1.5 text-faint hover:text-fg"
              onClick={() => setHelp((h) => !h)}
              title="Синтаксис"
            >
              <HelpCircle size={13} />
            </button>
          </div>
          <input
            className="field font-mono text-[12.5px]"
            value={draft.labels}
            spellCheck={false}
            placeholder="app=nginx, env in (prod,stage)"
            onChange={(e) => set({ labels: e.target.value })}
          />
          {help && (
            <div className="mt-2 space-y-1 rounded-lg border border-line bg-surface p-2.5 font-mono text-[11px] leading-relaxed text-muted">
              <div><span className="text-fg">key=value</span> · равно</div>
              <div><span className="text-fg">key!=value</span> · не равно / нет ключа</div>
              <div><span className="text-fg">key~part</span> · значение содержит</div>
              <div><span className="text-fg">key in (a,b)</span> · <span className="text-fg">notin (a,b)</span></div>
              <div><span className="text-fg">key</span> · есть ключ · <span className="text-fg">!key</span> · нет</div>
              <div className="pt-1 font-sans text-faint">Условия через запятую объединяются по И.</div>
            </div>
          )}
        </div>

        <div>
          <label className="label">Состояние</label>
          <div className="flex gap-1.5">
            <Toggle checked={draft.pending} onChange={(v) => onApply({ ...draft, pending: v })}>
              Pending
            </Toggle>
            <Toggle checked={draft.deleting} tone="danger" onChange={(v) => onApply({ ...draft, deleting: v })}>
              Deleting
            </Toggle>
          </div>
        </div>

        <JsonFilterBuilder
          rows={draft.json}
          match={draft.json_match}
          baseState={draft}
          onChange={(json) => set({ json })}
          onMatchChange={(json_match) => set({ json_match })}
        />
      </div>

      <div className="flex gap-2 border-t border-line p-3">
        <Button type="submit" variant="primary" className="flex-1 justify-center">
          Применить <CornerDownLeft size={13} className="opacity-70" />
        </Button>
        <Button size="icon" className="h-9 w-9" onClick={onReset} title="Сбросить всё">
          <RotateCcw size={14} />
        </Button>
      </div>
    </form>
  );
}
