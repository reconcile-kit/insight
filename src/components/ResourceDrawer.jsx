import React, { useCallback, useEffect, useRef, useState } from "react";
import { X, RefreshCw, Save, Trash2, Copy, Check, AlertTriangle, Loader2, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { JsonEditor } from "@/components/JsonEditor";
import { KindChip, GroupLabel, LabelChip } from "@/components/Chips";
import { useToast } from "@/components/Toaster";
import { getResource, updateResource, createResource, deleteResource } from "@/lib/api";
import { cn, fmtDate, timeAgo } from "@/lib/utils";

const pretty = (v) => JSON.stringify(v, null, 2);

function Meta({ label, children, title }) {
  return (
    <div className="min-w-0" title={title}>
      <div className="text-[10.5px] uppercase tracking-wider text-faint">{label}</div>
      <div className="mt-0.5 truncate text-[13px] text-fg">{children}</div>
    </div>
  );
}

/**
 * Side panel for one resource. Every open re-fetches the full body from
 * state-manager (the list only carries a projection); saves and deletes go
 * there as well.
 */
export function ResourceDrawer({ target, onClose, onChanged, onOpen }) {
  const toast = useToast();
  const isCreate = target?.mode === "create";
  const [data, setData] = useState(null);
  const [text, setText] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [copied, setCopied] = useState(false);
  const loadedText = useRef("");

  const id = target?.id;
  const idKey = id ? `${id.resource_group}/${id.namespace}/${id.kind}/${id.name}` : "";

  const load = useCallback(
    async (signal) => {
      if (!id) return;
      setLoading(true);
      setError(null);
      try {
        const r = await getResource(id, signal);
        setData(r);
        loadedText.current = pretty(r);
        setText(loadedText.current);
      } catch (e) {
        if (e.name !== "AbortError") setError(e.message);
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [idKey]
  );

  useEffect(() => {
    setConfirmDelete(false);
    setError(null);
    if (!target) return;
    if (isCreate) {
      setData(null);
      loadedText.current = pretty(target.template);
      setText(loadedText.current);
      return;
    }
    setData(null);
    setText("");
    const ctl = new AbortController();
    load(ctl.signal);
    return () => ctl.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target, load]);

  const dirty = text !== loadedText.current;

  const close = useCallback(() => {
    if (dirty && !isCreate && !window.confirm("Есть несохранённые изменения. Закрыть?")) return;
    onClose();
  }, [dirty, isCreate, onClose]);

  useEffect(() => {
    if (!target) return;
    const onKey = (e) => {
      if (e.key === "Escape" && !e.defaultPrevented) close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [target, close]);

  const parse = () => {
    try {
      return JSON.parse(text);
    } catch (e) {
      toast(`Некорректный JSON: ${e.message}`, "error");
      return null;
    }
  };

  const save = async () => {
    const body = parse();
    if (!body || saving) return;
    setSaving(true);
    try {
      if (isCreate) {
        const created = await createResource(body);
        toast(`Ресурс ${body.name} создан`);
        onChanged();
        const r = created && created.name ? created : body;
        onOpen({ resource_group: r.resource_group, namespace: r.namespace, kind: r.kind, name: r.name });
      } else {
        const updated = await updateResource(id, body);
        if (updated && typeof updated === "object") {
          setData(updated);
          loadedText.current = pretty(updated);
          setText(loadedText.current);
        } else {
          await load();
        }
        toast("Сохранено");
        onChanged();
      }
    } catch (e) {
      toast(e.message, "error");
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    if (!confirmDelete) {
      setConfirmDelete(true);
      setTimeout(() => setConfirmDelete(false), 4000);
      return;
    }
    try {
      await deleteResource(id);
      toast(`Ресурс ${id.name} удалён`);
      onChanged();
      onClose();
    } catch (e) {
      toast(e.message, "error");
    }
  };

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      toast("Не удалось скопировать", "error");
    }
  };

  if (!target) return null;

  const pending = data && data.version > data.current_version;
  const labels = Object.entries(data?.labels || {}).sort(([a], [b]) => a.localeCompare(b));

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-[2px] animate-in fade-in-0 duration-200" onMouseDown={close} />
      <aside className="relative flex h-full w-[min(820px,100vw)] flex-col border-l border-line bg-surface shadow-2xl animate-in slide-in-from-right-8 fade-in-0 duration-200">
        <header className="flex items-start gap-3 border-b border-line px-5 py-4">
          <div className="min-w-0 flex-1">
            {isCreate ? (
              <div className="flex items-center gap-2 text-base font-semibold">
                <Plus size={16} className="text-accent" /> Новый ресурс
              </div>
            ) : (
              <>
                <div className="flex items-center gap-2">
                  <KindChip kind={id.kind} group={id.resource_group} />
                  <GroupLabel group={id.resource_group} className="text-xs text-muted" />
                  <span className="text-xs text-faint">/</span>
                  <span className="truncate text-xs text-muted">{id.namespace}</span>
                </div>
                <h2 className="mt-1.5 truncate font-mono text-[17px] font-semibold tracking-tight">{id.name}</h2>
              </>
            )}
          </div>
          <div className="flex items-center gap-1">
            {!isCreate && (
              <>
                <Button size="icon" variant="ghost" onClick={copy} title="Копировать JSON">
                  {copied ? <Check size={15} className="text-ok" /> : <Copy size={15} />}
                </Button>
                <Button
                  size="icon"
                  variant="ghost"
                  onClick={() => (!dirty || window.confirm("Отменить изменения и перезагрузить?")) && load()}
                  title="Перезагрузить из state-manager"
                >
                  <RefreshCw size={15} className={cn(loading && "animate-spin")} />
                </Button>
              </>
            )}
            <Button size="icon" variant="ghost" onClick={close} title="Закрыть (Esc)">
              <X size={16} />
            </Button>
          </div>
        </header>

        {data && (
          <div className="space-y-3 border-b border-line px-5 py-3">
            <div className="grid grid-cols-4 gap-4">
              <Meta label="Shard">
                <span className="font-mono text-[12.5px]">{data.shard_id}</span>
              </Meta>
              <Meta label="Version" title="current_version / version">
                <span className={cn("font-mono text-[12.5px]", pending ? "text-warn" : "text-fg")}>
                  {data.current_version} / {data.version}
                  {pending && <span className="ml-1.5 text-[11px] font-sans">pending</span>}
                </span>
              </Meta>
              <Meta label="Created" title={fmtDate(data.created_at)}>
                {timeAgo(data.created_at)} <span className="text-faint">назад</span>
              </Meta>
              <Meta label="Updated" title={fmtDate(data.updated_at)}>
                {timeAgo(data.updated_at)} <span className="text-faint">назад</span>
              </Meta>
            </div>
            {data.deletion_timestamp && (
              <div className="flex items-center gap-2 rounded-lg border border-danger/25 bg-danger/10 px-3 py-2 text-xs text-danger">
                <AlertTriangle size={13} /> Помечен на удаление {fmtDate(data.deletion_timestamp)}
                {data.finalizers?.length > 0 && <span className="text-danger/80">· ждёт финализаторы: {data.finalizers.join(", ")}</span>}
              </div>
            )}
            {labels.length > 0 && (
              <div className="flex flex-wrap gap-1">
                {labels.map(([k, v]) => (
                  <LabelChip key={k} k={k} v={v} />
                ))}
              </div>
            )}
          </div>
        )}

        <div className="relative min-h-0 flex-1 bg-bg/40">
          {loading && !data && (
            <div className="absolute inset-0 flex items-center justify-center text-sm text-muted">
              <Loader2 size={18} className="mr-2 animate-spin" /> Загрузка из state-manager…
            </div>
          )}
          {error && (
            <div className="m-5 rounded-xl border border-danger/30 bg-danger/10 p-4 text-sm text-danger">
              <div className="mb-1 flex items-center gap-2 font-medium">
                <AlertTriangle size={15} /> Не удалось загрузить ресурс
              </div>
              <div className="font-mono text-xs opacity-90">{error}</div>
            </div>
          )}
          {(data || isCreate) && <JsonEditor value={text} onChange={setText} onSave={save} />}
        </div>

        <footer className="flex items-center gap-2 border-t border-line px-5 py-3">
          {!isCreate && (
            <Button variant="danger" onClick={remove} disabled={!data}>
              <Trash2 size={14} /> {confirmDelete ? "Точно удалить?" : "Удалить"}
            </Button>
          )}
          <div className="flex-1 text-xs text-faint">
            {dirty && !isCreate && <span className="text-warn">● Изменено</span>}
          </div>
          <span className="hidden items-center gap-1 text-xs text-faint sm:flex">
            <span className="kbd">⌘</span>
            <span className="kbd">S</span>
          </span>
          <Button variant="primary" onClick={save} disabled={saving || (!isCreate && (!data || !dirty))}>
            {saving ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
            {isCreate ? "Создать" : "Сохранить"}
          </Button>
        </footer>
      </aside>
    </div>
  );
}
