import React, { useState, useEffect } from "react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { ResourceModal } from "@/components/ui/modal";
import SyntaxHighlighter from "react-syntax-highlighter";
import { dracula } from "react-syntax-highlighter/dist/esm/styles/prism";

// API requests are proxied in dev and prod
const API_BASE = import.meta.env.VITE_API_BASE || "/api";

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
  const [error, setError] = useState(null);
  const [refreshFlag, setRefreshFlag] = useState(0);
  const [loading, setLoading] = useState(false);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  
  // Пагинация
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [totalCount, setTotalCount] = useState(0);
  const [hasNextPage, setHasNextPage] = useState(false);
  const limit = 100;

  const handleChange = (field) => (e) =>
      setFilters((f) => ({ ...f, [field]: e.target.value }));

  const listResources = async (page = currentPage) => {
    setLoading(true);
    setError(null);

    try {
      const params = new URLSearchParams();
      Object.entries(filters).forEach(([k, v]) => {
        if (v) params.append(k, v);
      });
      
      // Добавляем параметры пагинации
      const offset = (page - 1) * limit;
      params.append('limit', limit);
      params.append('offset', offset);

      let url = `${API_BASE}/api/v1/resources?${params}`;
      let res = await fetch(url);
      
      // Если API не поддерживает пагинацию (404), пробуем без параметров пагинации
      if (!res.ok && res.status === 404) {
        const paramsWithoutPagination = new URLSearchParams();
        Object.entries(filters).forEach(([k, v]) => {
          if (v) paramsWithoutPagination.append(k, v);
        });
        url = `${API_BASE}/api/v1/resources?${paramsWithoutPagination}`;
        res = await fetch(url);
      }
      
      if (!res.ok) throw new Error(`Ошибка ${res.status}`);
      const data = await res.json();
      
      // Обрабатываем ответ API
      if (data && typeof data === 'object' && 'items' in data) {
        // API поддерживает пагинацию
        const items = Array.isArray(data.items) ? data.items : [];
        const total = data.total || 0;
        
        setResources(items);
        setTotalCount(total);
        setTotalPages(Math.ceil(total / limit));
        
        // Определяем, есть ли следующая страница
        const hasMore = items.length === limit;
        setHasNextPage(hasMore);
      } else {
        // API не поддерживает пагинацию - показываем все данные
        const items = Array.isArray(data) ? data : [data];
        
        setResources(items);
        setTotalCount(items.length);
        setTotalPages(1);
        setHasNextPage(false);
      }
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  // Функции пагинации
  const goToNextPage = () => {
    const newPage = currentPage + 1;
    setCurrentPage(newPage);
    listResources(newPage);
  };

  const goToPrevPage = () => {
    if (currentPage > 1) {
      const newPage = currentPage - 1;
      setCurrentPage(newPage);
      listResources(newPage);
    }
  };

  const resetPagination = () => {
    setCurrentPage(1);
    setRefreshFlag((f) => f + 1);
  };

  useEffect(() => {
    listResources();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshFlag]);

  const openEditModal = (r) => {
    setSelected(r);
    document.body.style.overflow = "hidden";
  };

  const closeEditModal = () => {
    setSelected(null);
    document.body.style.overflow = "auto";
  };

  const openCreateModal = () => {
    setIsCreateModalOpen(true);
    document.body.style.overflow = "hidden";
  };

  const closeCreateModal = () => {
    setIsCreateModalOpen(false);
    document.body.style.overflow = "auto";
  };

  const updateResource = async (resource) => {
    if (!selected) return;

    const { resource_group, kind, namespace, name } = resource;

    try {
      const res = await fetch(
          `${API_BASE}/api/v1/groups/${resource_group}/namespaces/${namespace}/kinds/${kind}/resources/${name}`,
          {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(resource),
          }
      );
      if (!res.ok) throw new Error(`Ошибка ${res.status}`);
      const data = await res.json();
      setSelected(data);
    } catch (e) {
      setError(e.message);
    }
  };

  const deleteResource = async () => {
    if (!selected) return;

    const { resource_group, kind, namespace, name } = selected;

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
      closeEditModal();
    } catch (e) {
      setError(e.message);
    }
  };

  const createResource = async (resource) => {
    const { resource_group, kind, namespace } = resource;
    try {
      const res = await fetch(`${API_BASE}/api/v1/groups/${resource_group}/namespaces/${namespace}/kinds/${kind}/resources/`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(resource),
      });
      if (!res.ok) throw new Error(`Ошибка ${res.status}`);
      setRefreshFlag((f) => f + 1);
      closeCreateModal();
    } catch (e) {
      setError(e.message);
    }
  };

  const refreshResource = async () => {
    if (!selected) return;

    const { resource_group, kind, namespace, name } = selected;
    try {
      const res = await fetch(
          `${API_BASE}/api/v1/groups/${resource_group}/namespaces/${namespace}/kinds/${kind}/resources/${name}`
      );
      if (!res.ok) throw new Error(`Ошибка ${res.status}`);
      const data = await res.json();
      setSelected(data);
    } catch (e) {
      setError(e.message);
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
          <Button onClick={resetPagination}>Применить</Button>
        </div>
        <div className="flex-1 flex flex-col">
          <div className="flex justify-between items-center space-x-4 border-b px-4 py-2">
            {/* Пагинация слева */}
            <div className="flex items-center space-x-2">
              <Button 
                onClick={goToPrevPage} 
                disabled={currentPage <= 1}
                variant="outline"
                size="sm"
              >
                ← Предыдущая
              </Button>
              <span className="text-xs text-gray-500">
                Offset: {(currentPage - 1) * limit}
              </span>
              <Button 
                onClick={goToNextPage} 
                variant="outline"
                size="sm"
              >
                Следующая →
              </Button>
            </div>
            
            {/* Индикатор загрузки и ошибок посередине */}
            <div className="flex-1 flex justify-center">
              {loading && <p className="text-sm text-gray-600">Загрузка...</p>}
              {error && <p className="text-sm text-red-500">{error}</p>}
            </div>
            
            {/* Кнопки справа */}
            <div className="flex space-x-4">
              <Button onClick={openCreateModal}>Добавить</Button>
              <Button onClick={resetPagination}>Обновить</Button>
            </div>
          </div>
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
                      onClick={() => openEditModal(r)}
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
        <ResourceModal
            mode="edit"
            isOpen={!!selected}
            onClose={closeEditModal}
            initialJson={selected ? JSON.stringify(selected, null, 2) : ""}
            onSubmit={updateResource}
            onRefresh={refreshResource}
            onDelete={deleteResource}
        />
        <ResourceModal
            mode="create"
            isOpen={isCreateModalOpen}
            onClose={closeCreateModal}
            initialJson=""
            onSubmit={createResource}
        />
      </div>
  );
}