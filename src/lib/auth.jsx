import React, { createContext, useContext, useEffect, useState } from "react";
import { Loader2 } from "lucide-react";

const LEVELS = { none: 0, read: 1, write: 2, admin: 3 };

/** Login is checked by the backend; this only mirrors it in the UI. */
const AuthContext = createContext({ auth: false, user: null, access: "admin" });

export const useAuth = () => useContext(AuthContext);

/** can(access, "write") — whether a level includes another. */
export const can = (access, level) => (LEVELS[access] ?? 0) >= LEVELS[level];

export function loginUrl(returnTo = window.location.pathname + window.location.search) {
  return `/login?return_to=${encodeURIComponent(returnTo)}`;
}

/** The session expired while the page was open: go through the login page again. */
export function toLogin() {
  window.location.assign(loginUrl());
}

export async function fetchMe() {
  const res = await fetch("/auth/me", { headers: { Accept: "application/json" } });
  const body = await res.json().catch(() => ({}));
  return { status: res.status, ...body };
}

/** Renders children once the user is known; sends anonymous users to /login. */
export function AuthGate({ children, noAccess }) {
  const [me, setMe] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchMe()
      .then((m) => {
        if (m.status === 401) return toLogin();
        if (m.status !== 200) throw new Error(`/auth/me: ${m.status}`);
        setMe({ auth: !!m.auth, user: m.user || null, access: m.access || "none" });
      })
      .catch((e) => setError(e.message));
  }, []);

  if (error) {
    return (
      <div className="flex h-full items-center justify-center p-6 text-sm text-danger">Не удалось проверить вход: {error}</div>
    );
  }
  if (!me) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted">
        <Loader2 size={18} className="mr-2 animate-spin" /> Загрузка…
      </div>
    );
  }
  return (
    <AuthContext.Provider value={me}>{me.auth && me.access === "none" ? noAccess(me) : children}</AuthContext.Provider>
  );
}
