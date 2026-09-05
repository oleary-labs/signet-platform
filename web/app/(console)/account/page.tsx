"use client";

import { useEffect, useState } from "react";
import {
  Callout,
  CopyValue,
  ErrorNote,
  InfoTip,
  PageHeader,
  Section,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAction } from "@/lib/hooks";
import { useSession } from "@/providers/SessionProvider";
import { useToast } from "@/providers/ToastProvider";
import { formatDateTime } from "@/lib/format";

export default function AccountPage() {
  const { user, refresh } = useSession();
  const toast = useToast();
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");

  useEffect(() => {
    setDisplayName(user?.display_name ?? "");
    setEmail(user?.email ?? "");
  }, [user]);

  const [save, { pending, error }] = useAction(async () => {
    await api.updateMe({ display_name: displayName, email });
    await refresh();
    toast.success("Profile saved");
  });

  if (!user) return null;

  return (
    <>
      <PageHeader
        title="Account"
      />

      <div className="grid items-start gap-5 lg:grid-cols-3">
        <div className="space-y-5 lg:col-span-2">
          <Section title="Profile">
            <form
              className="space-y-5"
              onSubmit={(e) => {
                e.preventDefault();
                save();
              }}
            >
              <div>
                <label className="label" htmlFor="name">
                  Display name
                </label>
                <input
                  id="name"
                  className="input"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  placeholder="Your name"
                />
              </div>
              <div>
                <label className="label" htmlFor="email">
                  Email
                </label>
                <input
                  id="email"
                  type="email"
                  className="input"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="you@example.com"
                />
              </div>

              {error ? <ErrorNote error={error} /> : null}

              <button type="submit" disabled={pending} className="btn-primary">
                {pending ? "Saving…" : "Save profile"}
              </button>
            </form>
          </Section>

          <Section
            title="How you signed in"
          >
            <dl className="grid gap-x-8 gap-y-4 sm:grid-cols-2">
              <div>
                <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                  Route
                </dt>
                <dd className="mt-1 text-[13.5px] text-fg">
                  {user.subject_kind === "signet"
                    ? "Signet — threshold signature from the bootstrap group"
                    : "Sign-In with Ethereum"}
                </dd>
              </div>
              <div>
                <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                  Last seen
                </dt>
                <dd className="mt-1 text-[13.5px] text-fg">{formatDateTime(user.last_seen_at)}</dd>
              </div>
              <div className="min-w-0">
                <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                  Subject
                </dt>
                <dd className="mt-1">
                  <CopyValue value={user.subject} label="Subject" />
                </dd>
              </div>
              {user.account_address ? (
                <div className="min-w-0">
                  <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                    Account
                  </dt>
                  <dd className="mt-1">
                    <CopyValue value={user.account_address} label="Account address" />
                  </dd>
                </div>
              ) : null}
              {user.group_public_key ? (
                <div className="min-w-0 sm:col-span-2">
                  <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-faint">
                    Group public key
                  </dt>
                  <dd className="mt-1">
                    <CopyValue value={user.group_public_key} label="Group public key" />
                  </dd>
                </div>
              ) : null}
            </dl>
          </Section>
        </div>

        <div className="space-y-5">

          {user.is_staff ? (
            <Callout
              tone="accent"
              title={
                <>
                  Platform staff
                  <InfoTip>
                    Your account can curate the operator marketplace. That is separate from your
                    role in any organization — being staff grants no access to anyone&rsquo;s apps.
                  </InfoTip>
                </>
              }
            />
          ) : null}
        </div>
      </div>
    </>
  );
}
