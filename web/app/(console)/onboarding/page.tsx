"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { Logo } from "@/components/Logo";
import { ErrorNote } from "@/components/ui";
import { api } from "@/lib/api";
import { useAction } from "@/lib/hooks";
import { useSession } from "@/providers/SessionProvider";
import { useToast } from "@/providers/ToastProvider";

export default function OnboardingPage() {
  const router = useRouter();
  const { refresh, setActiveOrg, organizations } = useSession();
  const toast = useToast();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");

  const [create, { pending, error }] = useAction(async () => {
    const org = await api.createOrg(name.trim(), email.trim() || undefined);
    await refresh();
    setActiveOrg(org.id);
    toast.success(`${org.name} is ready`, "Create your first app to deploy a signing group.");
    router.replace("/apps/new");
  });

  // Someone who already has an org landed here by accident; send them on.
  if (organizations.length > 0) {
    router.replace("/apps");
    return null;
  }

  return (
    <div className="mx-auto max-w-lg py-8">
      <Logo size={38} className="text-fg" />
      <h1 className="mt-6 text-[26px] font-semibold tracking-tight text-fg">
        Create your organization
      </h1>
      <p className="mt-2.5 text-sm leading-relaxed text-muted">
        An organization owns your apps, your team, and your billing. You can rename it later, and
        you can belong to more than one.
      </p>

      <form
        className="mt-8 space-y-5"
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim()) create();
        }}
      >
        <div>
          <label className="label" htmlFor="org-name">
            Organization name
          </label>
          <input
            id="org-name"
            className="input"
            placeholder="Acme Labs"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
        </div>

        <div>
          <label className="label" htmlFor="org-email">
            Billing email <span className="normal-case text-faint">(optional)</span>
          </label>
          <input
            id="org-email"
            className="input"
            type="email"
            placeholder="billing@acme.dev"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>

        {error ? <ErrorNote error={error} /> : null}

        <button type="submit" disabled={pending || !name.trim()} className="btn-accent w-full py-3">
          {pending ? "Creating…" : "Create organization"}
        </button>
      </form>
    </div>
  );
}
