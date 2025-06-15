import React, { useState, useEffect } from "react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import SyntaxHighlighter from "react-syntax-highlighter";
import { dracula } from "react-syntax-highlighter/dist/esm/styles/prism";

// API requests are proxied in dev and prod
const API_BASE = "http://localhost:8080";

function timeAgo(ts) {
  if (!ts) return "-";
  const now = Date.now();
  const diff = now - new Date(ts).getTime();
  const minutes = Math.floor(diff / 60000);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  return `${days}d`;
}

export default function App() {
  const [filters, setFilters] = useState({
    resource_group: "",
    kind: "",
    shard_id: "",
    namespace: "",
  });

  const [resources, setResources] = useState([]);
  const [selected, setSelected] = useState(null);
  const [jsonInput, setJsonInput] = useState("");
  const [jsonError, setJsonError] = useState(null);
  const [error, setError] = useState(null);
  const [refreshFlag, setRefreshFlag] = useState(0);
  const [loading, setLoading] = useState(false);

  const handleChange = (field) => (e) =>
      setFilters((f) => ({ ...f, [field]: e.target.value }));

  const listResources = async () => {
    setLoading(true);
    setError(null);

    try {
      const params = new URLSearchParams();
      Object.entries(filters).forEach(([k, v]) => {
        if (v) params.append(k, v);
      });

      const res = await fetch(`${API_BASE}/api/v1/resources?${params}`);
      if (!res.ok) throw new Error(`Ошибка ${res.status}`);
      const data = await res.json();
      setResources(Array.isArray(data) ? data : [data]);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    listResources();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshFlag]);

  const open = (r) => {
    setSelected(r);
    setJsonInput(JSON.stringify(r, null, 2));
    setJsonError(null);
    document.body.style.overflow = 'hidden';
  };
  const close = () => {
    setSelected(null);
    setJsonInput("");
    setJsonError(null);
    document.body.style.overflow = 'auto';
  };

  const updateResource = async () => {
    if (jsonError || !selected) return;

    const { resource_group, kind, namespace, name } = selected;

    try {
      const res = await fetch(
          `${API_BASE}/api/v1/groups/${resource_group}/namespaces/${namespace}/kinds/${kind}/resources/${name}`,
          {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(selected),
          }
      );
      if (!res.ok) throw new Error(`Ошибка ${res.status}`);
      const data = await res.json();
      setSelected(data);
      setJsonInput(JSON.stringify(data, null, 2));
      setJsonError(null);
    } catch (e) {
      setError(e.message);
    }
  };

  const deleteResource = async () => {
    const { resource_group, kind, namespace, name } = selected;

    // Запрос подтверждения у пользователя
    const confirmed = window.confirm(
        `Вы уверены, что хотите удалить ресурс ${name} (группа: ${resource_group}, namespace: ${namespace}, kind: ${kind})?`
    );
    if (!confirmed) return;

    try {
      const res = await fetch(
          `${API_BASE}/api/v1/groups/${resource_group}/namespaces/${namespace}/kinds/${kind}/resources/${name}`,
          { method: "DELETE" }
      );
      if (!res.ok) throw new Error(`Ошибка ${res.status}`);
      setRefreshFlag((f) => f + 1);
      close();
    } catch (e) {
      setError(e.message);
    }
  };

  const refreshResource = async () => {
    const { resource_group, kind, namespace, name } = selected;
    try {
      const res = await fetch(
          `${API_BASE}/api/v1/groups/${resource_group}/namespaces/${namespace}/kinds/${kind}/resources/${name}`
      );
      if (!res.ok) throw new Error(`Ошибка ${res.status}`);
      const data = await res.json();
      setSelected(data);
      setJsonInput(JSON.stringify(data, null, 2));
      setJsonError(null);
    } catch (e) {
      setError(e.message);
    }
  };

  const handleJsonChange = (e) => {
    const input = e.target.value;
    setJsonInput(input);
    try {
      const parsed = JSON.parse(input);
      setSelected(parsed);
      setJsonError(null);
    } catch (err) {
      setJsonError("Некорректный JSON формат");
    }
  };

  return (
      <div className="h-screen flex">
        <div className="w-64 p-4 space-y-4 bg-gray-50 border-r">
          <Input
              placeholder="resource_group"
              value={filters.resource_group}
              onChange={handleChange("resource_group")}
          />
          <Input
              placeholder="kind"
              value={filters.kind}
              onChange={handleChange("kind")}
          />
          <Input
              className="w-48"
              placeholder="shard_id"
              value={filters.shard_id}
              onChange={handleChange("shard_id")}
          />
          <Input
              className="w-48"
              placeholder="namespace"
              value={filters.namespace}
              onChange={handleChange("namespace")}
          />
          <Button onClick={() => setRefreshFlag((f) => f + 1)}>Применить</Button>
        </div>
        <div className="flex-1 flex flex-col">
          <div className="p-4 flex justify-end space-x-4 border-b">

            <Button onClick={() => setRefreshFlag((f) => f + 1)}>Добавить</Button>
            <Button onClick={() => setRefreshFlag((f) => f + 1)}>Обновить</Button>
          </div>
          {loading && <p className="p-4">Загрузка...</p>}
          {error && <p className="p-4 text-red-500">{error}</p>}
          <div className="flex-1 overflow-auto">
            <table className="min-w-full text-sm">
              <thead className="sticky top-0 bg-gray-100">
              <tr>
                <th className="px-4 py-2 text-left">Name</th>
                <th className="px-4 py-2 text-left">Kind</th>
                <th className="px-4 py-2 text-left">Group</th>
                <th className="px-4 py-2 text-left">Age</th>
                <th className="px-4 py-2 text-left">Namespace</th>
                <th className="px-4 py-2 text-left">Shard</th>
              </tr>
              </thead>
              <tbody>
              {resources.map((r) => (
                  <tr
                      key={`${r.resource_group}-${r.namespace}-${r.kind}-${r.name}`}
                      className="hover:bg-gray-50 cursor-pointer"
                      onClick={() => open(r)}
                  >
                    <td className="border px-4 py-1">{r.name}</td>
                    <td className="border px-4 py-1">{r.kind}</td>
                    <td className="border px-4 py-1">{r.resource_group}</td>
                    <td className="border px-4 py-1">{timeAgo(r.created_at)}</td>
                    <td className="border px-4 py-1">{r.namespace}</td>
                    <td className="border px-4 py-1">{r.shard_id}</td>
                  </tr>
              ))}
              </tbody>
            </table>
          </div>
        </div>
        {selected && (
            <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
              <div className="bg-white rounded-2xl shadow-xl max-w-3xl w-full h-[90vh] flex flex-col">
                {/* тело модалки без собственного скролла */}
                <div className="p-4 flex-1 flex flex-col space-y-4 min-h-0">
                  {/* JSON-редактор */}
                  <div className="space-y-2 flex-1 flex flex-col min-h-0">
                    <h3 className="text-lg font-semibold">Редактировать JSON ресурса</h3>
                    <Button onClick={refreshResource}>Обновить данные</Button>

                    <textarea
                        className="w-full flex-1 min-h-0 p-2 border rounded font-mono text-sm
                     resize-none overflow-auto focus:outline-none"
                        value={jsonInput}
                        onChange={handleJsonChange}
                        placeholder="Введите JSON ресурса"
                    />
                    {jsonError && <p className="text-red-500 text-sm">{jsonError}</p>}
                  </div>
                </div>

                {/* нижняя панель */}
                <div className="p-4 flex justify-end space-x-2">
                  <Button onClick={updateResource} disabled={jsonError}>Сохранить</Button>
                  <Button variant="destructive" onClick={deleteResource}>Удалить</Button>
                  <Button variant="secondary" onClick={close}>Отмена</Button>
                </div>
              </div>
            </div>
        )}
      </div>
  );
}