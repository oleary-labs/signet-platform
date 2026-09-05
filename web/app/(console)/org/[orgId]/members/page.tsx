"use client";

import { use, useState } from "react";
import {
  Badge,
  Callout,
  CopyValue,
  EmptyState,
  ErrorNote,
  Modal,
  PageHeader,
  Section,
  SkeletonRows,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAction, useQuery } from "@/lib/hooks";
import { useSession } from "@/providers/SessionProvider";
import { useToast } from "@/providers/ToastProvider";
import { formatDate, shortAddress } from "@/lib/format";
import type { Role } from "@/lib/types";

const ROLES: { id: Role; label: string; description: string }[] = [
  { id: "owner", label: "Owner", description: "Everything, including billing and transferring ownership." },
  { id: "admin", label: "Admin", description: "Manage apps, members, and billing settings." },
  { id: "developer", label: "Developer", description: "Create and configure apps. Cannot manage members." },
  { id: "viewer", label: "Viewer", description: "Read-only across every app." },
];

export default function MembersPage({ params }: { params: Promise<{ orgId: string }> }) {
  const { orgId } = use(params);
  const toast = useToast();
  const { user } = useSession();

  const members = useQuery(() => api.members(orgId), [orgId]);
  const invites = useQuery(() => api.invites(orgId), [orgId]);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [inviteLink, setInviteLink] = useState<string | null>(null);

  const [setRole] = useAction(async (userId: string, role: Role) => {
    await api.setMemberRole(orgId, userId, role);
    await members.refresh();
    toast.success("Role updated");
  });

  const [remove] = useAction(async (userId: string) => {
    await api.removeMember(orgId, userId);
    await members.refresh();
    toast.success("Member removed");
  });

  const [revoke] = useAction(async (id: string) => {
    await api.revokeInvite(orgId, id);
    await invites.refresh();
    toast.success("Invitation revoked");
  });

  const pendingInvites = (invites.data ?? []).filter((i) => !i.accepted_at);

  return (
    <>
      <PageHeader
        title="Members"
        actions={
          <button type="button" className="btn-accent" onClick={() => setInviteOpen(true)}>
            Invite someone
          </button>
        }
      />

      <div className="space-y-5">
        <Section title={`Members (${members.data?.length ?? 0})`}>
          {members.error ? <ErrorNote error={members.error} onRetry={members.refresh} /> : null}
          {members.loading ? (
            <SkeletonRows rows={3} />
          ) : (
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>Person</th>
                    <th>Role</th>
                    <th>Joined</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {(members.data ?? []).map((m) => {
                    const isMe = m.user_id === user?.id;
                    return (
                      <tr key={m.user_id}>
                        <td>
                          <div className="min-w-0">
                            <p className="truncate text-[13.5px] font-medium text-fg">
                              {m.display_name || m.email || shortAddress(m.subject, 12, 6)}
                              {isMe ? <span className="ml-2 text-[12px] text-faint">you</span> : null}
                            </p>
                            {m.email && m.display_name ? (
                              <p className="truncate text-[12px] text-muted">{m.email}</p>
                            ) : null}
                          </div>
                        </td>
                        <td>
                          <select
                            className="select w-auto py-1.5 text-[13px]"
                            value={m.role}
                            onChange={(e) => setRole(m.user_id, e.target.value as Role)}
                          >
                            {ROLES.map((r) => (
                              <option key={r.id} value={r.id}>
                                {r.label}
                              </option>
                            ))}
                          </select>
                        </td>
                        <td className="whitespace-nowrap text-[13px] text-muted">
                          {formatDate(m.created_at)}
                        </td>
                        <td className="text-right">
                          <button
                            type="button"
                            className="btn-danger btn-sm"
                            onClick={() => remove(m.user_id)}
                          >
                            Remove
                          </button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}

        </Section>

        {pendingInvites.length > 0 ? (
          <Section title={`Pending invitations (${pendingInvites.length})`}>
            <ul className="space-y-2.5">
              {pendingInvites.map((i) => (
                <li
                  key={i.id}
                  className="flex flex-wrap items-center justify-between gap-3 rounded-xl border px-4 py-3 hairline"
                >
                  <div className="min-w-0">
                    <p className="truncate text-[13.5px] text-fg">{i.email}</p>
                    <p className="mt-0.5 text-[12px] text-muted">
                      {ROLES.find((r) => r.id === i.role)?.label} · expires {formatDate(i.expires_at)}
                    </p>
                  </div>
                  <div className="flex flex-none items-center gap-2">
                    <Badge tone="warn">pending</Badge>
                    <button type="button" className="btn-danger btn-sm" onClick={() => revoke(i.id)}>
                      Revoke
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          </Section>
        ) : null}

        <Section title="What each role can do">
          <ul className="space-y-2.5">
            {ROLES.map((r) => (
              <li key={r.id} className="flex gap-3 rounded-xl border px-4 py-3 hairline">
                <span className="w-24 flex-none text-[13.5px] font-medium text-fg">{r.label}</span>
                <span className="text-[13px] leading-relaxed text-muted">{r.description}</span>
              </li>
            ))}
          </ul>
        </Section>
      </div>

      <InviteModal
        open={inviteOpen}
        onClose={() => setInviteOpen(false)}
        orgId={orgId}
        onCreated={(url) => {
          setInviteOpen(false);
          invites.refresh();
          setInviteLink(url);
        }}
      />

      <Modal
        open={inviteLink !== null}
        onClose={() => setInviteLink(null)}
        title="Send this link"
        info="The platform does not send email, so nothing has been delivered yet — pass this on however you normally would."
        footer={
          <button type="button" className="btn-accent" onClick={() => setInviteLink(null)}>
            Done
          </button>
        }
      >
        <div className="rounded-xl border border-accent-500/40 bg-accent-500/[0.07] p-4">
          <CopyValue value={inviteLink ?? ""} className="w-full" />
        </div>
        <p className="mt-3 text-[13px] leading-relaxed text-muted">
          The link works once and expires in seven days. Anyone holding it can join with the role
          you chose, so treat it like a password.
        </p>
      </Modal>
    </>
  );
}

function InviteModal({
  open,
  onClose,
  orgId,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  orgId: string;
  onCreated: (url: string) => void;
}) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("developer");

  const [invite, { pending, error }] = useAction(async () => {
    const created = await api.createInvite(orgId, email.trim(), role);
    setEmail("");
    onCreated(created.accept_url ?? "");
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Invite someone"
      footer={
        <>
          <button type="button" className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            type="button"
            className="btn-accent"
            disabled={pending || !email.includes("@")}
            onClick={() => invite()}
          >
            {pending ? "Creating…" : "Create invitation"}
          </button>
        </>
      }
    >
      {error ? <ErrorNote error={error} /> : null}
      <label className="label mt-4" htmlFor="invite-email">
        Email
      </label>
      <input
        id="invite-email"
        type="email"
        className="input"
        placeholder="teammate@acme.dev"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
      />
      <div className="mt-5">
        <span className="label">Role</span>
        <div className="space-y-2">
          {ROLES.filter((r) => r.id !== "owner").map((r) => (
            <button
              key={r.id}
              type="button"
              onClick={() => setRole(r.id)}
              className={`w-full rounded-xl border px-3.5 py-2.5 text-left transition ${
                role === r.id ? "border-accent-500 bg-accent-500/[0.08]" : "hover:bg-fg/[0.03] hairline"
              }`}
            >
              <span className="block text-[13.5px] font-medium text-fg">{r.label}</span>
              <span className="mt-0.5 block text-[12px] text-muted">{r.description}</span>
            </button>
          ))}
        </div>
      </div>
    </Modal>
  );
}
