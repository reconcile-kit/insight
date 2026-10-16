import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Search, Plus, RefreshCw, Sun, Moon, X, PanelLeftClose, PanelLeft, AlertTriangle, Timer } from "lucide-react";
import { Button } from "@/components/ui/button";
import { FilterPanel } from "@/components/FilterPanel";
import { ResourceTable, rowKey } from "@/components/ResourceTable";
import { Pagination } from "@/components/Pagination";
import { ResourceDrawer } from "@/components/ResourceDrawer";
import { Toaster, useToast } from "@/components/Toaster";
import { Logo } from "@/components/Logo";
import { UserMenu } from "@/components/UserMenu";
import { useAuth, can } from "@/lib/auth";
import { searchResources } from "@/lib/api";
import { emptyState, readUrl, writeUrl, isJsonRowActive, JSON_OPS } from "@/lib/state";
import { cn, fmtNum, hue } from "@/lib/utils";

const AUTO_REFRESH = [0, 5, 15, 30, 60];

// Follows the system theme until the user picks one explicitly.
function useTheme() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
  }, [dark]);
  const toggle = () =>
    setDark((d) => {
      try {
        localStorage.setItem("insight.theme", d ? "light" : "dark");
      } catch {
        /* private mode */
      }
      return !d;
    });
  return [dark, toggle];
}

/** Removable chips describing the applied filters. */
function ActiveFilters({ state, onRemove, onClear }) {
  const chips = [];
  const add = (key, label, value, h) => chips.push({ key, label, value, h });
  if (state.name) add("name", "name ~", state.name);
  if (state.resource_group) add("resource_group", "group", state.resource_group, hue(state.resource_group));
  if (state.kind) add("kind", "kind", state.kind, state.resource_group ? hue(`${state.resource_group}/${state.kind}`) : undefined);
  if (state.namespace) add("namespace", "ns", state.namespace);
  if (state.shard_id) add("shard_id", "shard", state.shard_id);
  if (state.labels) add("labels", "labels", state.labels);
  if (state.pending) add("pending", "pending", "");
  if (state.deleting) add("deleting", "deleting", "");
  state.json.filter(isJsonRowActive).forEach((r) => {
    const op = JSON_OPS.find((o) => o.op === r.op)?.label || r.op;
    add(`json:${r.id}`, `${r.root}${r.path ? "." + r.path : ""}`, `${op} ${r.value ?? ""}`.trim());
  });
  if (!chips.length) return null;
  return (
    <div className="flex flex-wrap items-center gap-1.5 px-5 pb-3">
      {state.json.filter(isJsonRowActive).length > 1 && (
        <span className="mr-1 text-[10px] font-semibold uppercase tracking-wider text-muted">
          json: {state.json_match === "any" ? "или" : "и"}
        </span>
      )}
      {chips.map((c) => (
        <span
          key={c.key}
          className="inline-flex h-7 items-center gap-1.5 rounded-full border border-line bg-surface pl-3 pr-1 text-xs animate-in fade-in-0 zoom-in-95"
        >
          {c.h !== undefined && <span className="h-1.5 w-1.5 rounded-full" style={{ background: `hsl(${c.h} 70% 58%)` }} />}
          <span className="text-faint">{c.label}</span>
          {c.value && <span className="max-w-[280px] truncate font-mono text-[11.5px] text-fg">{c.value}</span>}
          <button
            onClick={() => onRemove(c.key)}
            className="flex h-5 w-5 items-center justify-center rounded-full text-faint hover:bg-line hover:text-fg"
          >
            <X size={11} />
          </button>
        </span>
      ))}
      <button onClick={onClear} className="ml-1 text-xs text-faint hover:text-fg">
        Сбросить всё
      </button>
    </div>
  );
}

function Insight() {
  const toast = useToast();
  const [dark, toggleTheme] = useTheme();
  const { access } = useAuth();
  const [applied, setApplied] = useState(readUrl);
  const [draft, setDraft] = useState(applied);
  const [result, setResult] = useState({ items: [], total: null });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [tick, setTick] = useState(0);
  const [drawer, setDrawer] = useState(null);
  const [sidebar, setSidebar] = useState(true);
  const [auto, setAuto] = useState(0);
  const searchRef = useRef(null);

  // Fetch on every applied change.
  useEffect(() => {
    writeUrl(applied);
    const ctl = new AbortController();
    setLoading(true);
    searchResources(applied, ctl.signal)
      .then((res) => {
        setResult({ items: res.items || [], total: res.total, countError: res.count_error });
        setError(null);
      })
      .catch((e) => {
        if (e.name !== "AbortError") setError(e.message);
      })
      .finally(() => {
        if (!ctl.signal.aborted) setLoading(false);
      });
    return () => ctl.abort();
  }, [applied, tick]);

  useEffect(() => {
    if (!auto) return;
    const t = setInterval(() => setTick((x) => x + 1), auto * 1000);
    return () => clearInterval(t);
  }, [auto]);

  const refresh = useCallback(() => setTick((x) => x + 1), []);

  /** Applies a full state (or the current draft) and resets to page 1. */
  const apply = useCallback(
    (next) => {
      const s = { ...(next || draft), page: 1 };
      setDraft(s);
      setApplied(s);
    },
    [draft]
  );

  const patchApplied = (patch) => {
    const s = { ...applied, ...patch, page: 1 };
    setDraft((d) => ({ ...d, ...patch }));
    setApplied(s);
  };

  const quickFilter = (field, value) => {
    patchApplied({ [field]: value });
    toast(`Фильтр: ${field} = ${value}`);
  };

  const addLabel = (term) => {
    const parts = applied.labels ? applied.labels.split(",").map((s) => s.trim()) : [];
    if (parts.includes(term)) return;
    patchApplied({ labels: [...parts, term].filter(Boolean).join(", ") });
  };

  const removeFilter = (key) => {
    if (key.startsWith("json:")) {
      const id = Number(key.slice(5));
      patchApplied({ json: applied.json.filter((r) => r.id !== id) });
    } else {
      patchApplied({ [key]: typeof emptyState[key] === "boolean" ? false : "" });
    }
  };

  const clearAll = () => {
    const s = { ...emptyState, size: applied.size, sort: applied.sort, desc: applied.desc };
    setDraft(s);
    setApplied(s);
  };

  const onSort = (field) => {
    // asc -> desc -> default order
    if (applied.sort !== field) setApplied({ ...applied, sort: field, desc: false, page: 1 });
    else if (!applied.desc) setApplied({ ...applied, desc: true, page: 1 });
    else setApplied({ ...applied, sort: "", desc: false, page: 1 });
  };

  const openCreate = () =>
    setDrawer({
      mode: "create",
      template: {
        resource_group: applied.resource_group || "",
        kind: applied.kind || "",
        namespace: applied.namespace || "",
        name: "",
        shard_id: applied.shard_id || "default",
        labels: {},
        annotations: {},
        spec: {},
      },
    });

  // Global shortcuts: ⌘K or / focuses search.
  useEffect(() => {
    const onKey = (e) => {
      const typing = /INPUT|TEXTAREA|SELECT/.test(document.activeElement?.tagName) || document.activeElement?.isContentEditable;
      if ((e.key === "k" && (e.metaKey || e.ctrlKey)) || (e.key === "/" && !typing && !drawer)) {
        e.preventDefault();
        searchRef.current?.focus();
        searchRef.current?.select();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [drawer]);

  const selected = drawer?.mode === "view" ? rowKey(drawer.id) : null;
  const total = result.total;

  const subtitle = useMemo(() => {
    if (error) return null;
    if (total == null) return result.countError ? "не удалось посчитать" : "…";
    return `${fmtNum(total)} ${plural(total, ["ресурс", "ресурса", "ресурсов"])}`;
  }, [total, error, result.countError]);

  return (
    <div className="flex h-full flex-col">
      {/* Top bar */}
      <header className="relative z-20 flex h-14 shrink-0 items-center gap-3 border-b border-line px-4">
        <button
          onClick={() => setSidebar((s) => !s)}
          className="flex h-8 w-8 items-center justify-center rounded-lg text-muted hover:bg-line/50 hover:text-fg"
          title="Фильтры"
        >
          {sidebar ? <PanelLeftClose size={16} /> : <PanelLeft size={16} />}
        </button>
        <div className="flex items-center gap-2.5 pr-3">
          <Logo size={30} className="text-fg" />
          <div className="leading-tight">
            <div className="text-sm font-semibold tracking-tight">Insight</div>
            <div className="text-[10.5px] text-faint">reconcile-kit</div>
          </div>
        </div>

        <form
          className="relative max-w-xl flex-1"
          onSubmit={(e) => {
            e.preventDefault();
            apply();
          }}
        >
          <Search size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-faint" />
          <input
            ref={searchRef}
            className="field h-9 rounded-xl bg-elevated pl-9 pr-16"
            placeholder="Поиск по имени ресурса…"
            value={draft.name}
            spellCheck={false}
            onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))}
            onKeyDown={(e) => {
              if (e.key === "Escape" && draft.name) {
                e.preventDefault();
                apply({ ...draft, name: "" });
              }
            }}
          />
          <span className="pointer-events-none absolute right-2.5 top-1/2 flex -translate-y-1/2 gap-0.5">
            <span className="kbd">⌘</span>
            <span className="kbd">K</span>
          </span>
        </form>

        <div className="ml-auto flex items-center gap-1.5">
          <label
            className={cn(
              "flex h-8 items-center gap-1 rounded-lg px-2 text-xs transition-colors",
              auto ? "bg-accent/10 text-accent" : "text-muted hover:bg-line/50"
            )}
            title="Автообновление"
          >
            <Timer size={14} />
            <select
              className="cursor-pointer appearance-none bg-transparent text-xs outline-none"
              value={auto}
              onChange={(e) => setAuto(Number(e.target.value))}
            >
              {AUTO_REFRESH.map((s) => (
                <option key={s} value={s}>
                  {s ? `${s}s` : "off"}
                </option>
              ))}
            </select>
          </label>
          <Button size="icon" variant="ghost" onClick={refresh} title="Обновить">
            <RefreshCw size={15} className={cn(loading && "animate-spin")} />
          </Button>
          <Button size="icon" variant="ghost" onClick={toggleTheme} title="Тема">
            {dark ? <Sun size={15} /> : <Moon size={15} />}
          </Button>
          {can(access, "write") && (
            <>
              <div className="mx-1 h-5 w-px bg-line" />
              <Button variant="primary" size="md" onClick={openCreate}>
                <Plus size={15} /> Создать
              </Button>
            </>
          )}
          <UserMenu />
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        {/* Sidebar */}
        <aside
          className={cn(
            "shrink-0 overflow-hidden border-r border-line bg-surface/40 transition-[width] duration-200",
            sidebar ? "w-[320px]" : "w-0 border-r-0"
          )}
        >
          <div className="h-full w-[320px]">
            <FilterPanel draft={draft} setDraft={setDraft} onApply={apply} onReset={clearAll} />
          </div>
        </aside>

        {/* Main */}
        <main className="flex min-w-0 flex-1 flex-col">
          <div className="flex items-baseline gap-3 px-5 pb-3 pt-4">
            <h1 className="text-lg font-semibold tracking-tight">Ресурсы</h1>
            {subtitle && <span className="text-sm tabular-nums text-muted">{subtitle}</span>}
            {loading && <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-accent" />}
          </div>
          <ActiveFilters state={applied} onRemove={removeFilter} onClear={clearAll} />

          {error && (
            <div className="mx-5 mb-3 flex items-start gap-2 rounded-xl border border-danger/30 bg-danger/10 px-3.5 py-2.5 text-sm text-danger">
              <AlertTriangle size={15} className="mt-0.5 shrink-0" />
              <span className="font-mono text-xs leading-relaxed">{error}</span>
            </div>
          )}

          <div className="mx-3 mb-3 flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-line bg-surface">
            <div className="min-h-0 flex-1 overflow-auto">
              <ResourceTable
                items={result.items}
                loading={loading}
                sort={applied.sort}
                desc={applied.desc}
                onSort={onSort}
                onFilter={quickFilter}
                onLabel={addLabel}
                onOpen={(r) =>
                  setDrawer({
                    mode: "view",
                    id: { resource_group: r.resource_group, namespace: r.namespace, kind: r.kind, name: r.name },
                  })
                }
                selected={selected}
              />
            </div>
            <Pagination
              page={applied.page}
              size={applied.size}
              total={total}
              shown={result.items.length}
              onPage={(page) => setApplied({ ...applied, page })}
              onSize={(size) => setApplied({ ...applied, size, page: 1 })}
            />
          </div>
        </main>
      </div>

      <ResourceDrawer
        target={drawer}
        onClose={() => setDrawer(null)}
        onChanged={refresh}
        onOpen={(id) => setDrawer({ mode: "view", id })}
      />
    </div>
  );
}

function plural(n, [one, few, many]) {
  const m10 = n % 10;
  const m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return one;
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return few;
  return many;
}

export default function App() {
  return (
    <Toaster>
      <Insight />
    </Toaster>
  );
}
