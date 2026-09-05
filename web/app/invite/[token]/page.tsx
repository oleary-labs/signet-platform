"use client";

import { useRouter } from "next/navigation";
import { use, useEffect, useState } from "react";
import { Logo } from "@/components/Logo";
import { ErrorNote } from "@/components/ui";
import { api } from "@/lib/api";
import { useSession } from "@/providers/SessionProvider";
import { useToast } from "@/providers/ToastProvider";

export default function InvitePage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = use(params);
  const router = useRouter();
  const toast = useToast();
  const { status, refresh, setActiveOrg } = useSession();
  const [error, setError] = useState<unknown>(null);
  const [accepting, setAccepting] = useState(false);

  useEffect(() => {
    if (status === "signed-out") {
      router.replace(`/login?next=${encodeURIComponent(`/invite/${token}`)}`);
    }
  }, [status, router, token]);

  const accept = async () => {
    setAccepting(true);
    setError(null);
    try {
      const org = await api.acceptInvite(token);
      await refresh();
      setActiveOrg(org.id);
      toast.success(`You have joined ${org.name}`);
      router.replace("/apps");
    } catch (err) {
      setError(err);
    } finally {
      setAccepting(false);
    }
  };

  if (status !== "signed-in") return null;

  return (
    <main className="flex min-h-screen items-center justify-center px-6">
      <div className="w-full max-w-md text-center">
        <Logo size={38} className="mx-auto text-fg" />
        <h1 className="mt-7 text-[22px] font-semibold tracking-tight text-fg">
          You have been invited
        </h1>
        <p className="mt-2.5 text-sm leading-relaxed text-muted">
          Accepting adds your account to the organization with the role the invitation carries.
        </p>

        {error ? (
          <div className="mt-6 text-left">
            <ErrorNote error={error} />
          </div>
        ) : null}

        <button type="button" className="btn-accent mt-7 w-full py-3" disabled={accepting} onClick={accept}>
          {accepting ? "Joining…" : "Accept invitation"}
        </button>
        <button type="button" className="btn-quiet mt-2 w-full" onClick={() => router.replace("/apps")}>
          Not now
        </button>
      </div>
    </main>
  );
}
