"use client";

import { useEffect, useState, type ReactNode } from "react";
import { ErrorNote } from "@/components/ui";
import { api } from "@/lib/api";
import { useAction, useQuery } from "@/lib/hooks";
import { useToast } from "@/providers/ToastProvider";
import type { AppSettings } from "@/lib/types";

/**
 * The shared shape of every configuration screen.
 *
 * Each screen owns exactly one section of the settings document and saves only
 * that section, so two people editing different screens never overwrite each
 * other. The save button stays disabled until something has actually changed —
 * a no-op save that reports success is worse than no button at all.
 */
export function SettingsForm<K extends keyof AppSettings>({
  appId,
  section,
  defaults,
  children,
  footer,
}: {
  appId: string;
  section: K;
  defaults: AppSettings[K];
  children: (value: AppSettings[K], set: (patch: Partial<AppSettings[K]>) => void) => ReactNode;
  footer?: ReactNode;
}) {
  const toast = useToast();
  const settings = useQuery(() => api.settings(appId), [appId]);
  const [value, setValue] = useState<AppSettings[K]>(defaults);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (!settings.data) return;
    const loaded = settings.data[section];
    setValue({ ...(defaults as object), ...(loaded as object) } as AppSettings[K]);
    setDirty(false);
    // `defaults` is a literal at each call site; re-running on it would reset
    // the form on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [settings.data, section]);

  const set = (patch: Partial<AppSettings[K]>) => {
    setValue((v) => ({ ...(v as object), ...(patch as object) }) as AppSettings[K]);
    setDirty(true);
  };

  const [save, { pending, error }] = useAction(async () => {
    await api.saveSettings(appId, section, value);
    await settings.refresh();
    setDirty(false);
    toast.success("Saved");
  });

  if (settings.error) return <ErrorNote error={settings.error} onRetry={settings.refresh} />;

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        save();
      }}
      className="space-y-5"
    >
      {children(value, set)}

      {error ? <ErrorNote error={error} /> : null}

      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" className="btn-accent" disabled={pending || !dirty}>
          {pending ? "Saving…" : dirty ? "Save changes" : "Saved"}
        </button>
        {dirty ? <span className="text-[13px] text-muted">You have unsaved changes.</span> : null}
        {footer}
      </div>
    </form>
  );
}
