"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { api, ApiError, storeToken } from "@/lib/api";
import { clearSigningSession } from "@/lib/session-key";
import type { NetworkConfig, Organization, Session, User } from "@/lib/types";

interface SessionState {
  status: "loading" | "signed-in" | "signed-out";
  user: User | null;
  organizations: Organization[];
  network: NetworkConfig | null;
  /** The org the console is currently scoped to. */
  activeOrg: Organization | null;
  setActiveOrg: (orgId: string) => void;
  refresh: () => Promise<void>;
  signOut: () => Promise<void>;
  onSignedIn: (session: Session, token: string) => void;
}

const SessionContext = createContext<SessionState | null>(null);

const ACTIVE_ORG_KEY = "signet_platform_active_org";

export function SessionProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<SessionState["status"]>("loading");
  const [user, setUser] = useState<User | null>(null);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [network, setNetwork] = useState<NetworkConfig | null>(null);
  const [activeOrgId, setActiveOrgId] = useState<string | null>(null);

  const apply = useCallback((session: Session) => {
    setUser(session.user);
    setOrganizations(session.organizations);
    setNetwork(session.network);
    setStatus("signed-in");

    // Keep the previously selected org if the user is still a member of it;
    // otherwise fall back to the first, so a removed member never lands on a
    // console scoped to an org they can no longer read.
    setActiveOrgId((current) => {
      const remembered = current ?? readStoredOrg();
      if (remembered && session.organizations.some((o) => o.id === remembered)) {
        return remembered;
      }
      return session.organizations[0]?.id ?? null;
    });
  }, []);

  const refresh = useCallback(async () => {
    try {
      apply(await api.me());
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        storeToken(null);
        setUser(null);
        setOrganizations([]);
        setStatus("signed-out");
        // The public pages still need the network config to render.
        api.config().then(setNetwork).catch(() => undefined);
        return;
      }
      throw err;
    }
  }, [apply]);

  useEffect(() => {
    refresh().catch(() => setStatus("signed-out"));
  }, [refresh]);

  const setActiveOrg = useCallback((orgId: string) => {
    setActiveOrgId(orgId);
    try {
      window.localStorage.setItem(ACTIVE_ORG_KEY, orgId);
    } catch {
      /* private-mode browsers throw; the selection just won't persist */
    }
  }, []);

  const signOut = useCallback(async () => {
    try {
      await api.logout();
    } finally {
      storeToken(null);
      clearSigningSession();
      setUser(null);
      setOrganizations([]);
      setActiveOrgId(null);
      setStatus("signed-out");
    }
  }, []);

  const onSignedIn = useCallback(
    (session: Session, token: string) => {
      storeToken(token);
      apply(session);
    },
    [apply],
  );

  const value = useMemo<SessionState>(
    () => ({
      status,
      user,
      organizations,
      network,
      activeOrg: organizations.find((o) => o.id === activeOrgId) ?? null,
      setActiveOrg,
      refresh,
      signOut,
      onSignedIn,
    }),
    [status, user, organizations, network, activeOrgId, setActiveOrg, refresh, signOut, onSignedIn],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

function readStoredOrg(): string | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage.getItem(ACTIVE_ORG_KEY);
  } catch {
    return null;
  }
}

export function useSession(): SessionState {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error("useSession must be used inside <SessionProvider>");
  return ctx;
}
