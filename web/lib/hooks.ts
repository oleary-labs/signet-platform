"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export interface Query<T> {
  data: T | null;
  error: unknown;
  loading: boolean;
  /** Refetch without clearing the current data, so a refresh does not blank
   *  the screen the user is reading. */
  refresh: () => Promise<void>;
  /** Replace the cached value locally after a mutation the server confirmed. */
  set: (updater: T | ((prev: T | null) => T)) => void;
}

/**
 * Fetch-on-mount with manual refresh.
 *
 * Deliberately small: the console's screens each read one or two endpoints, so
 * a cache layer would add more moving parts than it removes. `deps` behaves
 * like a dependency array — change it and the query re-runs.
 */
export function useQuery<T>(fetcher: () => Promise<T>, deps: unknown[] = []): Query<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);

  // Keep the latest fetcher without making it a dependency, so callers can
  // pass an inline closure without causing an infinite refetch loop.
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;

  // Guards against a slow first request resolving after a newer one and
  // overwriting fresher data, and against setting state after unmount.
  const generation = useRef(0);

  const run = useCallback(async (showSpinner: boolean) => {
    const mine = ++generation.current;
    if (showSpinner) setLoading(true);
    try {
      const result = await fetcherRef.current();
      if (generation.current !== mine) return;
      setData(result);
      setError(null);
    } catch (err) {
      if (generation.current !== mine) return;
      setError(err);
    } finally {
      if (generation.current === mine) setLoading(false);
    }
  }, []);

  useEffect(() => {
    run(true);
    return () => {
      generation.current++;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  const refresh = useCallback(() => run(false), [run]);

  const set = useCallback((updater: T | ((prev: T | null) => T)) => {
    setData((prev) => (typeof updater === "function" ? (updater as (p: T | null) => T)(prev) : updater));
  }, []);

  return { data, error, loading, refresh, set };
}

/**
 * Wrap an async action with pending state and error capture, so every button
 * in the console disables itself while its request is in flight instead of
 * letting an impatient double-click submit twice.
 */
export function useAction<Args extends unknown[], R>(
  action: (...args: Args) => Promise<R>,
): [(...args: Args) => Promise<R | undefined>, { pending: boolean; error: unknown }] {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const inFlight = useRef(false);

  const run = useCallback(
    async (...args: Args) => {
      if (inFlight.current) return undefined;
      inFlight.current = true;
      setPending(true);
      setError(null);
      try {
        return await action(...args);
      } catch (err) {
        setError(err);
        return undefined;
      } finally {
        inFlight.current = false;
        setPending(false);
      }
    },
    [action],
  );

  return [run, { pending, error }];
}

/** Re-render on an interval, so countdowns tick without their own state. */
export function useTicker(intervalMs = 1000): number {
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const id = window.setInterval(() => setTick((t) => t + 1), intervalMs);
    return () => window.clearInterval(id);
  }, [intervalMs]);
  return tick;
}
