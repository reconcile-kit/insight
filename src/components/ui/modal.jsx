import React, { useState, useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";

function ResourceModal({
                           mode,
                           isOpen,
                           onClose,
                           initialJson,
                           onSubmit,
                           onRefresh,
                           onDelete,
                       }) {
    const [jsonInput, setJsonInput] = useState(initialJson);
    const [jsonError, setJsonError] = useState(null);
    const modalRef = useRef(null);

    useEffect(() => {
        setJsonInput(initialJson);
        setJsonError(null);
    }, [initialJson]);

    const handleJsonChange = (e) => {
        const input = e.target.value;
        setJsonInput(input);
        try {
            JSON.parse(input);
            setJsonError(null);
        } catch (err) {
            setJsonError("Некорректный JSON формат");
        }
    };

    const handleOverlayMouseDown = (e) => {
        if (modalRef.current && !modalRef.current.contains(e.target)) {
            onClose();
        }
    };

    const handleSubmit = () => {
        if (jsonError) return;
        try {
            const parsed = JSON.parse(jsonInput);
            onSubmit(parsed);
        } catch (err) {
            setJsonError("Некорректный JSON формат");
        }
    };

    if (!isOpen) return null;

    return (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50" onMouseDown={handleOverlayMouseDown}>
            <div ref={modalRef} className="bg-white rounded-2xl shadow-xl max-w-3xl w-full h-[90vh] flex flex-col">
                <div className="p-4 flex-1 flex flex-col space-y-4 min-h-0">
                    <div className="space-y-2 flex-1 flex flex-col min-h-0">
                        <h3 className="text-lg font-semibold">
                            {mode === "create" ? "Создать новый ресурс" : "Редактировать JSON ресурса"}
                        </h3>
                        {mode === "edit" && <Button onClick={onRefresh}>Обновить данные</Button>}
                        <textarea
                            className="w-full flex-1 min-h-0 p-2 border rounded font-mono text-sm resize-none overflow-auto focus:outline-none"
                            value={jsonInput}
                            onChange={handleJsonChange}
                            placeholder={
                                mode === "create"
                                    ? 'Введите JSON ресурса, например: {"name": "resource1", "kind": "type1", "resource_group": "group1", "namespace": "ns1"}'
                                    : "Введите JSON ресурса"
                            }
                        />
                        {jsonError && <p className="text-red-500 text-sm">{jsonError}</p>}
                    </div>
                </div>
                <div className="p-4 flex justify-end space-x-2">
                    <Button onClick={handleSubmit} disabled={jsonError}>
                        {mode === "create" ? "Добавить" : "Сохранить"}
                    </Button>
                    {mode === "edit" && (
                        <Button variant="destructive" onClick={onDelete}>
                            Удалить
                        </Button>
                    )}
                    <Button variant="secondary" onClick={onClose}>
                        Отмена
                    </Button>
                </div>
            </div>
        </div>
    );
}

export {ResourceModal};