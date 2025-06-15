import React, { useState, useEffect } from "react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";

// Use environment variable for API base URL
const API_BASE = import.meta.env.VITE_API_BASE || "";

/**
 * Переводит timestamp (created_at) в человекочитаемый возраст ресурса
 * 10m, 3h, 5d и т.д.
 */
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
  // Состояние фильтров
  const [filters, setFilters] = useState({
    resource_group: "",
    kind: "",
    shard_id: "",
    namespace: "",
  });

  // Состояние списка ресурсов и выбранного ресурса
  const [resources, setResources] = useState([]);
  const [selected, setSelected] = useState(null);

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [refreshFlag, setRefreshFlag] = useState(0); // инкрементируем чтобы триггерить useEffect

  /**
   * Обработчик изменения любого фильтра
   */
  const handleChange = (field) => (e) =>
    setFilters((f) => ({ ...f, [field]: e.target.value }));

  /**
   * Получает список ресурсов согласно текущим фильтрам
   */
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

  /**
   * Запрашиваем список при монтировании и всякий раз, когда refreshFlag меняется
   */
  useEffect(() => {
    listResources();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshFlag]);

  // Открыть ресурс в pop-up
  const open = (r) => setSelected(r);
  const close = () => setSelected(null);

  /** CRUD операции над выбранным ресурсом */
  const updateResource = async () => {
    const { resource_group, kind, namespace, name } = selected;

    await fetch(
      `${API_BASE}/api/v1/groups/${resource_group}/namespaces/${namespace}/kinds/${kind}/resources/${name}`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(selected),
      }
    );

    setRefreshFlag((f) => f + 1);
    close();
  };

  const deleteResource = async () => {
    const { resource_group, kind, namespace, name } = selected;

    await fetch(
      `${API_BASE}/api/v1/groups/${resource_group}/namespaces/${namespace}/kinds/${kind}/resources/${name}`,
      { method: "DELETE" }
    );

    setRefreshFlag((f) => f + 1);
    close();
  };

  const refreshResource = async () => {
    const { resource_group, kind, namespace, name } = selected;
    const res = await fetch(
      `${API_BASE}/api/v1/groups/${resource_group}/namespaces/${namespace}/kinds/${kind}/resources/${name}`
    );
    const data = await res.json();
    setSelected(data);
  };

  return (
    <div className="h-screen flex">
      {/* Левая колонка с двумя input */}
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

        {/* Кнопка применяет фильтр, по сути триггерит повторный запрос */}
        <Button onClick={() => setRefreshFlag((f) => f + 1)}>Применить</Button>
      </div>

      {/* Правая часть: верхняя панель и таблица */}
      <div className="flex-1 flex flex-col">
        {/* Верхняя правая панель с ещё двумя input */}
        <div className="p-4 flex justify-end space-x-4 border-b">
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
          <Button onClick={() => setRefreshFlag((f) => f + 1)}>Обновить</Button>
        </div>

        {loading && <p className="p-4">Загрузка...</p>}
        {error && <p className="p-4 text-red-500">{error}</p>}

        {/* Таблица ресурсов */}
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

      {/* Popup / модальное окно для выбранного ресурса */}
      {selected && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
          <div className="bg-white rounded-2xl overflow-hidden shadow-xl max-w-3xl w-full max-h-[90vh] flex flex-col">
            <div className="p-4 overflow-auto flex-1">
              <pre>{JSON.stringify(selected, null, 2)}</pre>
            </div>
            <div className="p-4 flex justify-end space-x-2">
              <Button onClick={updateResource}>Update</Button>
              <Button variant="destructive" onClick={deleteResource}>
                Delete
              </Button>
              <Button onClick={refreshResource}>Refresh</Button>
              <Button variant="secondary" onClick={close}>
                Cancel
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}