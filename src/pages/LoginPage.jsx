import React, { useEffect, useRef } from "react";
import { AlertTriangle, LogOut } from "lucide-react";
import { Logo } from "@/components/Logo";
import { fetchMe } from "@/lib/auth";

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

/** Drifting points that glow near the pointer and now and then on their own. */
function Particles() {
  const ref = useRef(null);

  useEffect(() => {
    const canvas = ref.current;
    const ctx = canvas?.getContext("2d");
    if (!canvas || !ctx) return;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)");
    const pointer = { x: -1000, y: -1000 };
    let width = 0;
    let height = 0;
    let frame = 0;
    let points = [];

    const resize = () => {
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      width = window.innerWidth;
      height = window.innerHeight;
      canvas.width = width * dpr;
      canvas.height = height * dpr;
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      const count = Math.min(76, Math.max(30, Math.round((width * height) / 21000)));
      const now = performance.now();
      points = Array.from({ length: count }, (_, i) => ({
        x: Math.random() * width,
        y: Math.random() * height,
        vx: (Math.random() - 0.5) * 0.18,
        vy: (Math.random() - 0.5) * 0.18,
        r: 1 + Math.random() * 1.8,
        phase: i * 0.37 + Math.random() * 2,
        glowAt: now + 600 + Math.random() * 12000,
        glowFor: 1000 + Math.random() * 700,
      }));
    };

    const ownGlow = (p, t) => {
      if (t < p.glowAt) return 0;
      const k = (t - p.glowAt) / p.glowFor;
      if (k >= 1) {
        p.glowAt = t + 4000 + Math.random() * 12000;
        p.glowFor = 1000 + Math.random() * 700;
        return 0;
      }
      return Math.sin(k * Math.PI) * 0.95;
    };

    const draw = (t) => {
      ctx.clearRect(0, 0, width, height);
      for (const p of points) {
        if (reduced.matches) break;
        p.x += p.vx;
        p.y += p.vy;
        if (p.x < -20 || p.x > width + 20) p.vx *= -1;
        if (p.y < -20 || p.y > height + 20) p.vy *= -1;
      }
      for (const p of points) {
        const near = Math.max(0, 1 - Math.hypot(p.x - pointer.x, p.y - pointer.y) / 180);
        const g = Math.max(near, ownGlow(p, t));
        const halo = 42 + g * 24;
        const grad = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, halo);
        grad.addColorStop(0, `rgba(139, 124, 255, ${0.14 + g * 0.12})`);
        grad.addColorStop(1, "rgba(139, 124, 255, 0)");
        ctx.fillStyle = grad;
        ctx.beginPath();
        ctx.arc(p.x, p.y, halo, 0, Math.PI * 2);
        ctx.fill();
        ctx.fillStyle = `rgba(74, 222, 128, ${0.32 + g * 0.55})`;
        ctx.shadowColor = "rgba(74, 222, 128, 0.8)";
        ctx.shadowBlur = 10 + g * 14;
        ctx.beginPath();
        ctx.arc(p.x, p.y, p.r + g * 2 + Math.sin(t * 0.00045 + p.phase) * 0.35, 0, Math.PI * 2);
        ctx.fill();
        ctx.shadowBlur = 0;
      }
      if (!reduced.matches) frame = requestAnimationFrame(draw);
    };

    const onMove = (e) => {
      pointer.x = e.clientX;
      pointer.y = e.clientY;
    };
    const onLeave = () => {
      pointer.x = -1000;
      pointer.y = -1000;
    };
    resize();
    window.addEventListener("resize", resize);
    window.addEventListener("pointermove", onMove, { passive: true });
    window.addEventListener("pointerleave", onLeave);
    if (reduced.matches) draw(0);
    else frame = requestAnimationFrame(draw);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("resize", resize);
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerleave", onLeave);
    };
  }, []);

  return <canvas ref={ref} className="pointer-events-none fixed inset-0 opacity-90" aria-hidden="true" />;
}

/** Full-screen dark backdrop with the hero text and a centered card. */
export function LoginShell({ children }) {
  useEffect(() => {
    // The login screen is always dark, whatever the app theme is.
    const root = document.documentElement;
    const was = root.classList.contains("dark");
    root.classList.add("dark");
    return () => root.classList.toggle("dark", was);
  }, []);

  return (
    <main className="relative flex min-h-full items-center overflow-hidden bg-[#06070c] px-5 py-8 text-[#e8eaf6] sm:px-8 lg:px-12">
      <Particles />
      <div
        className="pointer-events-none fixed inset-0 bg-[radial-gradient(circle_at_50%_42%,rgba(139,124,255,0.13),transparent_32rem)]"
        aria-hidden="true"
      />
      <div
        className="pointer-events-none fixed inset-0 bg-[linear-gradient(90deg,rgba(6,7,12,0.8),transparent_42%,rgba(6,7,12,0.65))]"
        aria-hidden="true"
      />
      <div className="relative mx-auto grid w-full max-w-[1160px] items-center gap-12 2xl:grid-cols-[minmax(0,1fr)_420px] 2xl:gap-2">
        <section className="hidden max-w-3xl 2xl:block">
          <div className="mb-8 flex items-center gap-3 text-[11px] font-semibold uppercase tracking-[0.3em] text-violet-200">
            <span className="h-2 w-2 rounded-full bg-[#4ade80] shadow-[0_0_18px_rgba(74,222,128,0.65)]" />
            reconcile-kit
          </div>
          <h1 className="text-7xl font-semibold leading-[0.98] tracking-[-0.06em] text-white">
            Every resource,
            <span className="block">
              <span className="inline-block whitespace-nowrap bg-gradient-to-r from-violet-300 via-indigo-300 to-[#4ade80] bg-clip-text pb-2 text-transparent">
                in sight
              </span>
            </span>
          </h1>
          <p className="mt-8 max-w-md text-base leading-7 text-slate-400">
            <span className="block">Поиск, фильтры и правка ресурсов state-manager в одном окне.</span>
            <span className="mt-2 block">
              Insight читает из реплики базы и меняет ресурсы только через API state-manager.
            </span>
          </p>
        </section>
        <section className="relative w-full max-w-[420px] justify-self-center overflow-hidden rounded-[30px] border border-white/10 bg-[rgba(12,13,24,0.82)] p-7 text-center shadow-[0_32px_110px_rgba(0,0,0,0.45),inset_0_1px_0_rgba(255,255,255,0.08)] backdrop-blur-2xl backdrop-saturate-150 sm:p-10">
          <div
            className="pointer-events-none absolute inset-0 opacity-80 [background-image:linear-gradient(rgba(139,124,255,0.06)_1px,transparent_1px),linear-gradient(90deg,rgba(139,124,255,0.06)_1px,transparent_1px)] [background-size:34px_34px] [mask-image:linear-gradient(to_bottom,black,transparent_78%)]"
            aria-hidden="true"
          />
          <div className="relative flex flex-col items-center gap-6">{children}</div>
        </section>
      </div>
    </main>
  );
}

function Brand() {
  return (
    <>
      <Logo size={80} className="text-white drop-shadow-[0_0_24px_rgba(139,124,255,0.45)]" />
      <h2 className="text-4xl font-bold tracking-[-0.045em] text-white">Insight</h2>
    </>
  );
}

const pill =
  "inline-flex min-w-[220px] items-center justify-center gap-2 rounded-full px-5 py-2.5 text-[13px] font-semibold transition";

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
      <Brand />
      <p className="max-w-xs text-sm leading-6 text-slate-400">
        Войдите через корпоративную учётную запись, чтобы открыть ресурсы state-manager.
      </p>
      {error && (
        <div className="flex w-full items-start gap-2 rounded-2xl border border-red-400/25 bg-red-500/10 px-4 py-3 text-left text-[13px] leading-5 text-red-200">
          <AlertTriangle size={15} className="mt-0.5 shrink-0" />
          <span>{ERRORS[error] || "Не удалось войти. Попробуйте ещё раз."}</span>
        </div>
      )}
      <a
        href={`/auth/login?return_to=${encodeURIComponent(returnTo)}`}
        className={`${pill} bg-[#8b7cff] text-white shadow-lg shadow-[rgba(139,124,255,0.3)] hover:bg-[#9d90ff]`}
      >
        Продолжить через SSO
      </a>
    </LoginShell>
  );
}

/** Shown to a logged-in user whose roles grant no access. */
export function NoAccessPage({ user }) {
  return (
    <LoginShell>
      <Brand />
      <p className="max-w-xs text-sm leading-6 text-slate-400">
        У учётной записи <span className="font-medium text-white">{user?.email || user?.name}</span> нет доступа к
        insight. Попросите администратора выдать роль на чтение.
      </p>
      <a href="/auth/logout" className={`${pill} border border-white/15 bg-white/5 text-white hover:bg-white/10`}>
        <LogOut size={14} /> Войти под другой учётной записью
      </a>
    </LoginShell>
  );
}
