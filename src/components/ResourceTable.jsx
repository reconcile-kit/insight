import React from "react";
import { ArrowDown, ArrowUp, ChevronsUpDown, Filter, Trash2, Inbox } from "lucide-react";
import { KindChip, GroupLabel, LabelChip } from "@/components/Chips";
import { cn, hue, timeAgo, fmtDate } from "@/lib/utils";

export const rowKey = (r) => `${r.resource_group}/${r.namespace}/${r.kind}/${r.name}`;

const COLS = [
  { key: "name", label: "Name", sort: "name", w: "min-w-[240px]" },
  { key: "kind", label: "Kind", sort: "kind", w: "w-[1%]" },
  { key: "resource_group", label: "Group", sort: "resource_group", w: "w-[1%]" },
  { key: "namespace", label: "Namespace", sort: "namespace", w: "w-[1%]" },
  { key: "shard_id", label: "Shard", sort: "shard_id", w: "w-[1%]" },
  { key: "labels", label: "Labels", w: "min-w-[180px]" },
  { key: "version", label: "Ver", sort: "version", w: "w-[1%]", right: true },
  { key: "updated_at", label: "Updated", sort: "updated_at", w: "w-[1%]", right: true },
  { key: "created_at", label: "Age", sort: "created_at", w: "w-[1%]", right: true },
];

/** Cell value that adds itself as a filter on click of the funnel icon. */
function Filterable({ onFilter, children, title }) {
  return (
    <span className="group/f inline-flex max-w-full items-center gap-1">
      <span className="min-w-0 truncate">{children}</span>
      <button
        type="button"
        title={title}
        onClick={(e) => {
          e.stopPropagation();
          onFilter();
        }}
        className="flex h-5 w-5 shrink-0 items-center justify-center rounded text-faint opacity-0 transition-opacity hover:bg-accent/15 hover:text-accent group-hover/f:opacity-100"
      >
        <Filter size={11} />
      </button>
    </span>
  );
}

function SortIcon({ active, desc }) {
  if (!active) return <ChevronsUpDown size={12} className="opacity-0 transition-opacity group-hover/h:opacity-60" />;
  return desc ? <ArrowDown size={12} className="text-accent" /> : <ArrowUp size={12} className="text-accent" />;
}

export function ResourceTable({ items, loading, sort, desc, onSort, onFilter, onLabel, onOpen, selected }) {
  return (
    <table className="w-full border-separate border-spacing-0 text-[13px]">
      <thead className="sticky top-0 z-10">
        <tr className="glass">
          {COLS.map((c) => (
            <th
              key={c.key}
              className={cn(
                "whitespace-nowrap border-b border-line px-3 py-2 text-left text-[11px] font-medium uppercase tracking-wider text-faint first:pl-5",
                c.w,
                c.right && "text-right"
              )}
            >
              {c.sort ? (
                <button
                  type="button"
                  onClick={() => onSort(c.sort)}
                  className={cn(
                    "group/h inline-flex items-center gap-1 uppercase transition-colors hover:text-fg",
                    sort === c.sort && "text-fg"
                  )}
                  title="Сортировать"
                >
                  {c.label}
                  <SortIcon active={sort === c.sort} desc={desc} />
                </button>
              ) : (
                c.label
              )}
            </th>
          ))}
        </tr>
      </thead>
      <tbody className={cn("transition-opacity", loading && items.length > 0 && "opacity-60")}>
        {items.length === 0 && loading &&
          Array.from({ length: 12 }).map((_, i) => (
            <tr key={i}>
              {COLS.map((c) => (
                <td key={c.key} className="border-b border-line/60 px-3 py-3 first:pl-5">
                  <div className="relative h-3 overflow-hidden rounded bg-line/50" style={{ width: c.key === "name" ? "70%" : "60%" }}>
                    <div className="absolute inset-0 -translate-x-full animate-shimmer bg-gradient-to-r from-transparent via-fg/5 to-transparent" />
                  </div>
                </td>
              ))}
            </tr>
          ))}
        {items.length === 0 && !loading && (
          <tr>
            <td colSpan={COLS.length} className="py-24 text-center text-muted">
              <Inbox className="mx-auto mb-3 text-faint" size={28} strokeWidth={1.5} />
              Ничего не найдено
              <div className="mt-1 text-xs text-faint">Попробуйте ослабить фильтры</div>
            </td>
          </tr>
        )}
        {items.map((r) => {
          const key = rowKey(r);
          const pending = r.version > r.current_version;
          const labels = Object.entries(r.labels || {}).sort(([a], [b]) => a.localeCompare(b));
          return (
            <tr
              key={key}
              onClick={() => onOpen(r)}
              className={cn(
                "group cursor-pointer transition-colors hover:bg-line/30",
                selected === key && "bg-accent/[0.07] hover:bg-accent/10"
              )}
            >
              <td
                className="relative border-b border-line/60 py-1.5 pl-5 pr-3"
                style={{ "--h": hue(`${r.resource_group}/${r.kind}`) }}
              >
                <span
                  className="absolute bottom-1 left-0 top-1 w-[3px] rounded-r-full opacity-70 transition-opacity group-hover:opacity-100"
                  style={{ background: "hsl(var(--h) 70% 58%)" }}
                />
                <span className="flex items-center gap-2">
                  {r.deletion_timestamp && (
                    <span title={`Удаляется с ${fmtDate(r.deletion_timestamp)}`}>
                      <Trash2 size={12} className="shrink-0 text-danger" />
                    </span>
                  )}
                  <span className={cn("truncate font-mono text-[12.5px] font-medium", r.deletion_timestamp && "text-muted line-through decoration-danger/50")}>
                    {r.name}
                  </span>
                </span>
              </td>
              <td className="whitespace-nowrap border-b border-line/60 px-3 py-1.5">
                <Filterable onFilter={() => onFilter("kind", r.kind)} title="Фильтр по kind">
                  <KindChip kind={r.kind} group={r.resource_group} />
                </Filterable>
              </td>
              <td className="max-w-[220px] whitespace-nowrap border-b border-line/60 px-3 py-1.5 text-muted">
                <Filterable onFilter={() => onFilter("resource_group", r.resource_group)} title="Фильтр по group">
                  <GroupLabel group={r.resource_group} />
                </Filterable>
              </td>
              <td className="max-w-[200px] whitespace-nowrap border-b border-line/60 px-3 py-1.5 text-muted">
                <Filterable onFilter={() => onFilter("namespace", r.namespace)} title="Фильтр по namespace">
                  {r.namespace}
                </Filterable>
              </td>
              <td className="whitespace-nowrap border-b border-line/60 px-3 py-1.5 font-mono text-[12px] text-muted">
                <Filterable onFilter={() => onFilter("shard_id", r.shard_id)} title="Фильтр по shard">
                  {r.shard_id}
                </Filterable>
              </td>
              <td className="border-b border-line/60 px-3 py-1.5">
                <div className="flex max-w-[420px] flex-wrap gap-1">
                  {labels.slice(0, 3).map(([k, v]) => (
                    <LabelChip
                      key={k}
                      k={k}
                      v={v}
                      title={`Фильтр ${k}=${v}`}
                      onClick={(e) => {
                        e.stopPropagation();
                        onLabel(`${k}=${v}`);
                      }}
                    />
                  ))}
                  {labels.length > 3 && (
                    <span
                      className="inline-flex h-5 items-center rounded-md px-1.5 text-[10.5px] text-faint"
                      title={labels.slice(3).map(([k, v]) => `${k}=${v}`).join("\n")}
                    >
                      +{labels.length - 3}
                    </span>
                  )}
                </div>
              </td>
              <td className="whitespace-nowrap border-b border-line/60 px-3 py-1.5 text-right font-mono text-[12px]">
                <span
                  title={`version ${r.version} · current_version ${r.current_version}`}
                  className={cn("inline-flex items-center gap-1.5", pending ? "text-warn" : "text-muted")}
                >
                  {pending && <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-warn" />}
                  {r.current_version}/{r.version}
                </span>
              </td>
              <td className="whitespace-nowrap border-b border-line/60 px-3 py-1.5 text-right tabular-nums text-muted" title={fmtDate(r.updated_at)}>
                {timeAgo(r.updated_at)}
              </td>
              <td className="whitespace-nowrap border-b border-line/60 px-3 py-1.5 pr-5 text-right tabular-nums text-muted" title={fmtDate(r.created_at)}>
                {timeAgo(r.created_at)}
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
