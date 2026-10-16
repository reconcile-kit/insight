import React, { useEffect, useRef, useState } from "react";
import { LogOut } from "lucide-react";
import { useAuth } from "@/lib/auth";
import { cn } from "@/lib/utils";

const ACCESS = {
  read: { label: "только чтение", cls: "bg-line/60 text-muted" },
  write: { label: "чтение и запись", cls: "bg-accent/15 text-accent" },
  admin: { label: "полный доступ", cls: "bg-ok/15 text-ok" },
};

const initials = (s = "") =>
  s
    .split(/[\s.@_-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0].toUpperCase())
    .join("") || "?";

/** Avatar with the user's name, access level and logout. Hidden when login is off. */
export function UserMenu() {
  const { auth, user, access } = useAuth();
  const [open, setOpen] = useState(false);
  const ref = useRef(null);

  useEffect(() => {
    if (!open) return;
    const close = (e) => !ref.current?.contains(e.target) && setOpen(false);
    const esc = (e) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);

  if (!auth || !user) return null;
  const a = ACCESS[access] || ACCESS.read;

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((o) => !o)}
        title={user.name}
        className="flex h-8 w-8 items-center justify-center rounded-full bg-gradient-to-br from-accent to-ok text-[11px] font-semibold text-white ring-2 ring-transparent transition hover:ring-accent/30"
      >
        {initials(user.name || user.email)}
      </button>
      {open && (
        <div className="absolute right-0 top-10 z-50 w-64 rounded-xl border border-line bg-elevated p-3 shadow-float animate-in fade-in-0 zoom-in-95 duration-150">
          <div className="truncate text-sm font-medium text-fg">{user.name}</div>
          {user.email && user.email !== user.name && <div className="truncate text-xs text-muted">{user.email}</div>}
          <span className={cn("mt-2 inline-block rounded-md px-1.5 py-0.5 text-[11px] font-medium", a.cls)}>{a.label}</span>
          <div className="my-3 h-px bg-line" />
          <a
            href="/auth/logout"
            className="flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm text-fg transition hover:bg-line/50"
          >
            <LogOut size={14} /> Выйти
          </a>
        </div>
      )}
    </div>
  );
}
