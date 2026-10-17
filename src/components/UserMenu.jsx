import React, { useEffect, useRef, useState } from "react";
import { LogOut, User } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/lib/auth";
import { cn } from "@/lib/utils";

const ACCESS = {
  read: { label: "read only", cls: "bg-line/60 text-muted" },
  write: { label: "read & write", cls: "bg-accent/15 text-accent" },
  admin: { label: "full access", cls: "bg-ok/15 text-ok" },
};

/** User button with the user's name, access level and logout. Hidden when login is off. */
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
      <Button size="icon" variant="ghost" onClick={() => setOpen((o) => !o)} title={user.name || user.email}>
        <User size={15} />
      </Button>
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
            <LogOut size={14} /> Sign out
          </a>
        </div>
      )}
    </div>
  );
}
