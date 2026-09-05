"use client";

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";

type ToastKind = "success" | "error" | "info";

interface Toast {
  id: number;
  kind: ToastKind;
  message: string;
  detail?: string;
}

interface ToastApi {
  success: (message: string, detail?: string) => void;
  error: (message: string, detail?: string) => void;
  info: (message: string, detail?: string) => void;
}

const ToastContext = createContext<ToastApi | null>(null);

let nextId = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const push = useCallback((kind: ToastKind, message: string, detail?: string) => {
    const id = nextId++;
    setToasts((t) => [...t, { id, kind, message, detail }]);
    // Errors linger: a developer who looked away should still find out why a
    // save failed.
    const ttl = kind === "error" ? 9000 : 4200;
    window.setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), ttl);
  }, []);

  const api = useMemo<ToastApi>(
    () => ({
      success: (m, d) => push("success", m, d),
      error: (m, d) => push("error", m, d),
      info: (m, d) => push("info", m, d),
    }),
    [push],
  );

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div
        className="pointer-events-none fixed bottom-5 right-5 z-[100] flex w-[min(380px,calc(100vw-40px))] flex-col gap-2.5"
        role="status"
        aria-live="polite"
      >
        {toasts.map((t) => (
          <div
            key={t.id}
            className={`pointer-events-auto anim-rise rounded-xl border px-4 py-3 shadow-pop backdrop-blur ${
              t.kind === "error"
                ? "border-error-500/35 bg-error-50 text-error-600 dark:bg-error-500/15 dark:text-error-400"
                : t.kind === "success"
                  ? "border-success-500/35 bg-success-50 text-success-700 dark:bg-success-500/15 dark:text-success-400"
                  : "border-edge/15 bg-surface text-fg"
            }`}
          >
            <p className="text-sm font-semibold">{t.message}</p>
            {t.detail ? <p className="mt-1 text-[13px] opacity-80">{t.detail}</p> : null}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used inside <ToastProvider>");
  return ctx;
}
