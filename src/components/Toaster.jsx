import React, { createContext, useCallback, useContext, useState } from "react";
import { CheckCircle2, AlertTriangle, X } from "lucide-react";
import { cn } from "@/lib/utils";

const Ctx = createContext(() => {});
export const useToast = () => useContext(Ctx);

let id = 0;

export function Toaster({ children }) {
  const [toasts, setToasts] = useState([]);
  const dismiss = (tid) => setToasts((t) => t.filter((x) => x.id !== tid));
  const push = useCallback((text, type = "ok") => {
    const tid = ++id;
    setToasts((t) => [...t.slice(-3), { id: tid, text, type }]);
    setTimeout(() => dismiss(tid), type === "error" ? 8000 : 3500);
  }, []);

  return (
    <Ctx.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed bottom-5 right-5 z-[60] flex w-[380px] flex-col gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            className={cn(
              "pointer-events-auto flex items-start gap-2.5 rounded-xl border bg-elevated/95 p-3 text-sm shadow-float backdrop-blur animate-in fade-in-0 slide-in-from-bottom-2",
              t.type === "error" ? "border-danger/30" : "border-line"
            )}
          >
            {t.type === "error" ? (
              <AlertTriangle size={16} className="mt-0.5 shrink-0 text-danger" />
            ) : (
              <CheckCircle2 size={16} className="mt-0.5 shrink-0 text-ok" />
            )}
            <span className="min-w-0 flex-1 break-words">{t.text}</span>
            <button className="text-faint hover:text-fg" onClick={() => dismiss(t.id)}>
              <X size={14} />
            </button>
          </div>
        ))}
      </div>
    </Ctx.Provider>
  );
}
