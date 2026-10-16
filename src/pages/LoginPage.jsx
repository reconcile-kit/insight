import React, { useEffect, useRef, useState } from "react";
import { AlertTriangle, LogIn, LogOut } from "lucide-react";
import { Logo } from "@/components/Logo";
import { fetchMe } from "@/lib/auth";
import { cn, hue } from "@/lib/utils";

const ERRORS = {
  state_expired: "Время на вход истекло. Попробуйте ещё раз.",
  state_mismatch: "Не удалось проверить ответ провайдера. Попробуйте ещё раз.",
  provider_error: "Провайдер отклонил вход.",
  provider_unavailable: "Провайдер входа сейчас недоступен. Попробуйте позже.",
  exchange_failed: "Не удалось подтвердить вход у провайдера.",
  invalid_token: "Провайдер вернул некорректный токен.",
  internal: "Внутренняя ошибка при входе.",
};

const safeReturnTo = (p) => (p && p.startsWith("/") && !p.startsWith("//") && !p.startsWith("/login") ? p : "/");

// Demo kinds for the map; colored with the same hue() as kind chips in the table.
const KINDS = [
  "compute.example.io/Instance",
  "compute.example.io/Disk",
  "network.example.io/Network",
  "network.example.io/LoadBalancer",
  "storage.example.io/Bucket",
  "jobs.example.io/Job",
  "jobs.example.io/CronJob",
  "dns.example.io/Record",
];

const SYNCED = 0;
const PENDING = 1; // spec changed: version > current_version
const RECONCILING = 2; // a controller is on it
const DELETING = 3;
const READY = 4; // just converged, shown green for a moment

const TILE = 22; // px, including the gap

/**
 * A live map of resources: every tile is a resource colored by its kind.
 * Now and then a spec changes (pending), a controller picks it up
 * (reconciling) and it converges back to synced: the reconcile loop insight
 * shows you.
 */
function ResourceMap() {
  const ref = useRef(null);
  const [size, setSize] = useState({ cols: 0, rows: 0 });
  const [tiles, setTiles] = useState([]);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => {
      const cols = Math.max(1, Math.floor(e.contentRect.width / TILE));
      const rows = Math.max(1, Math.floor(e.contentRect.height / TILE));
      setSize((s) => (s.cols === cols && s.rows === rows ? s : { cols, rows }));
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  useEffect(() => {
    const n = size.cols * size.rows;
    setTiles(
      Array.from({ length: n }, () => {
        const r = Math.random();
        return {
          kind: KINDS[Math.floor(Math.random() * KINDS.length)],
          state: r < 0.012 ? PENDING : r < 0.02 ? RECONCILING : SYNCED,
          since: Date.now() - Math.random() * 3000,
          empty: Math.random() < 0.18,
        };
      })
    );
  }, [size]);

  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const id = setInterval(() => {
      const now = Date.now();
      setTiles((prev) => {
        if (!prev.length) return prev;
        const next = prev.slice();
        // New spec changes arrive.
        // A calm cluster: a few changes at a time, not a light show.
        for (let k = 0; k < Math.max(1, Math.round(next.length / 450)); k++) {
          const i = Math.floor(Math.random() * next.length);
          const t = next[i];
          if (t.empty) {
            if (Math.random() < 0.15) next[i] = { ...t, empty: false, state: PENDING, since: now }; // created
          } else if (t.state === SYNCED) {
            next[i] = { ...t, state: Math.random() < 0.06 ? DELETING : PENDING, since: now };
          }
        }
        // Controllers move resources along the loop.
        for (let i = 0; i < next.length; i++) {
          const t = next[i];
          const age = now - t.since;
          if (t.state === PENDING && age > 1600 + (i % 7) * 300) next[i] = { ...t, state: RECONCILING, since: now };
          else if (t.state === RECONCILING && age > 1400 + (i % 5) * 350) next[i] = { ...t, state: READY, since: now };
          else if (t.state === READY && age > 1300) next[i] = { ...t, state: SYNCED, since: now };
          else if (t.state === DELETING && age > 2600) next[i] = { ...t, empty: true, state: SYNCED, since: now };
        }
        return next;
      });
    }, 600);
    return () => clearInterval(id);
  }, []);

  return (
    <div className="relative flex h-full flex-col">
      <div ref={ref} className="relative min-h-0 flex-1 overflow-hidden">
        <div
          className="grid gap-[6px]"
          style={{ gridTemplateColumns: `repeat(${size.cols}, ${TILE - 6}px)`, gridAutoRows: `${TILE - 6}px` }}
        >
          {tiles.map((t, i) => (
            <span
              key={i}
              className={cn(
                "rounded-[5px] transition-[background-color,box-shadow,opacity,transform] duration-700",
                t.empty && "opacity-0",
                t.state === RECONCILING && "animate-pulse"
              )}
              style={tileStyle(t)}
            />
          ))}
        </div>
        {/* Fade the map into the panel edges. */}
        <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_center,transparent_35%,rgb(var(--bg))_85%)]" />
      </div>

      <PhaseTicker />
    </div>
  );
}

function tileStyle(t) {
  const h = hue(t.kind);
  switch (t.state) {
    case PENDING:
      return { background: "rgb(var(--warn) / 0.85)", boxShadow: "0 0 14px rgb(var(--warn) / 0.45)" };
    case RECONCILING:
      return { background: "rgb(var(--accent))", boxShadow: "0 0 16px rgb(var(--accent) / 0.55)" };
    case READY:
      return { background: "rgb(var(--ok))", boxShadow: "0 0 16px rgb(var(--ok) / 0.55)" };
    case DELETING:
      return { background: "rgb(var(--danger) / 0.75)", transform: "scale(0.7)" };
    default:
      return {
        background: `hsl(${h} 65% var(--chip-l) / 0.22)`,
        boxShadow: `inset 0 0 0 1px hsl(${h} 65% var(--chip-l) / 0.35)`,
      };
  }
}

// Slow and soft: the phases change unhurriedly.
// The fade is part of the interval: hold (~4.4s) + fade.
const PHASE_EVERY = 5450; // ms between phases
const PHASE_FADE = { transitionDuration: "1050ms", transitionTimingFunction: "cubic-bezier(0.4, 0, 0.2, 1)" };

// The phases of the reconcile loop, lit one at a time. Illustrative only:
// no numbers, so nobody mistakes the demo map for real data.
const PHASES = [
  { word: "reconciling", color: "rgb(var(--accent))" },
  { word: "pending", color: "rgb(var(--warn))" },
  { word: "ready", color: "rgb(var(--ok))" },
  { word: "deleting", color: "rgb(var(--danger))" },
];

function PhaseTicker() {
  const [active, setActive] = useState(0);

  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const id = setInterval(() => setActive((a) => (a + 1) % PHASES.length), PHASE_EVERY);
    return () => clearInterval(id);
  }, []);

  return (
    <div className="flex flex-wrap items-center gap-x-7 gap-y-2 px-1 pt-5 font-mono text-[12px]" aria-hidden="true">
      {PHASES.map((p, i) => {
        const on = i === active;
        return (
          <span
            key={p.word}
            className={cn(
              "flex items-center gap-2 transition-all",
              on ? "text-fg opacity-100 blur-0" : "text-muted opacity-35 blur-[1.5px]"
            )}
            style={PHASE_FADE}
          >
            <span
              className="h-2 w-2 rounded-full transition-all"
              style={{
                ...PHASE_FADE,
                background: on ? p.color : "rgb(var(--faint))",
                boxShadow: on ? `0 0 10px ${p.color}` : "none",
              }}
            />
            {p.word}
          </span>
        );
      })}
    </div>
  );
}

/** Login column on the left, the live resource map on the right (wide screens). */
export function LoginShell({ children }) {
  return (
    <main className="grid min-h-full bg-bg text-fg lg:grid-cols-[minmax(420px,520px)_1fr]">
      <section className="relative flex flex-col justify-between border-line px-8 py-8 sm:px-12 lg:border-r lg:bg-surface">
        <div className="flex items-center gap-2.5 text-sm font-semibold">
          <Logo size={26} className="text-fg" />
          Insight
          <span className="font-normal text-faint">reconcile-kit</span>
        </div>
        <div className="my-16 w-full max-w-[360px]">{children}</div>
        <div className="text-xs leading-5 text-faint">
          Чтение — из реплики базы. Изменения — только через API state-manager.
        </div>
      </section>
      <section className="hidden min-h-0 flex-col p-10 lg:flex">
        <ResourceMap />
      </section>
    </main>
  );
}

const btn =
  "inline-flex h-11 w-full items-center justify-center gap-2 rounded-xl text-sm font-semibold transition focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-accent/25";

export default function LoginPage() {
  const params = new URLSearchParams(window.location.search);
  const returnTo = safeReturnTo(params.get("return_to"));
  const error = params.get("error");

  // Already logged in (or login is off): go straight to the app.
  useEffect(() => {
    fetchMe()
      .then((m) => {
        if (m.status === 200 && (!m.auth || m.access !== "none")) window.location.replace(returnTo);
      })
      .catch(() => {});
  }, [returnTo]);

  return (
    <LoginShell>
      <h1 className="text-[28px] font-semibold leading-tight tracking-tight">Вход в Insight</h1>
      <p className="mt-3 text-sm leading-6 text-muted">
        Ресурсы state-manager, их spec и status — с поиском по любому полю. Войдите корпоративной учётной записью.
      </p>
      {error && (
        <div className="mt-6 flex items-start gap-2 rounded-xl border border-danger/25 bg-danger/10 px-3.5 py-3 text-[13px] leading-5 text-danger">
          <AlertTriangle size={15} className="mt-0.5 shrink-0" />
          <span>{ERRORS[error] || "Не удалось войти. Попробуйте ещё раз."}</span>
        </div>
      )}
      <a
        href={`/auth/login?return_to=${encodeURIComponent(returnTo)}`}
        className={cn(btn, "mt-8 bg-accent text-accent-fg shadow-[0_8px_24px_-8px_rgb(var(--accent)/0.6)] hover:brightness-110")}
      >
        <LogIn size={16} /> Продолжить через SSO
      </a>
    </LoginShell>
  );
}

/** Shown to a logged-in user whose roles grant no access. */
export function NoAccessPage({ user }) {
  return (
    <LoginShell>
      <h1 className="text-[28px] font-semibold leading-tight tracking-tight">Нет доступа</h1>
      <p className="mt-3 text-sm leading-6 text-muted">
        У учётной записи <span className="font-medium text-fg">{user?.email || user?.name}</span> нет роли для
        insight. Попросите администратора выдать роль на чтение.
      </p>
      <a href="/auth/logout" className={cn(btn, "mt-8 border border-line bg-elevated text-fg hover:bg-line/40")}>
        <LogOut size={16} /> Войти под другой учётной записью
      </a>
    </LoginShell>
  );
}
