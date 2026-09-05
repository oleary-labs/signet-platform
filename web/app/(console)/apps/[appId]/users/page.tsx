"use client";

import { use, useState } from "react";
import {
  Badge,
  Callout,
  CopyValue,
  EmptyState,
  ErrorNote,
  PageHeader,
  Section,
  SkeletonRows,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAction, useQuery } from "@/lib/hooks";
import { useToast } from "@/providers/ToastProvider";
import { formatDateTime, relativeTime, shortHash } from "@/lib/format";

export default function UsersPage({ params }: { params: Promise<{ appId: string }> }) {
  const { appId } = use(params);
  const [search, setSearch] = useState("");
  const toast = useToast();

  const users = useQuery(
    () => api.appUsers(appId, { q: search || undefined, limit: "100" }),
    [appId, search],
  );

  const [label] = useAction(async (userId: string, value: string) => {
    await api.labelAppUser(appId, userId, value);
    await users.refresh();
    toast.success("Label saved");
  });

  return (
    <>
      <PageHeader
        title="Users"
        info="Nodes meter a hash of (issuer, subject), never the subject. The platform can count active wallets and group a person's keys, and genuinely cannot tell you who they are."
      /><Section
        title="Directory"
        actions={
          <input
            className="input w-56"
            placeholder="Search by hash or label…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            aria-label="Search users"
          />
        }
      >
        {users.error ? <ErrorNote error={users.error} onRetry={users.refresh} /> : null}

        {users.loading ? (
          <SkeletonRows rows={5} />
        ) : (users.data ?? []).length === 0 ? (
          <EmptyState
            title={search ? "No users match that search" : "No users yet"}
          />
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>User</th>
                  <th>Issuer</th>
                  <th>Keys</th>
                  <th>First seen</th>
                  <th>Last active</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {users.data!.map((u) => (
                  <tr key={u.id}>
                    <td>
                      <div className="min-w-0">
                        {u.label ? (
                          <p className="truncate text-[13.5px] font-medium text-fg">{u.label}</p>
                        ) : null}
                        <CopyValue
                          value={u.subject_hash}
                          display={shortHash(u.subject_hash, 12, 8)}
                          label="Subject hash"
                        />
                      </div>
                    </td>
                    <td className="text-[13px] text-muted">{u.issuer || "—"}</td>
                    <td>{u.key_count}</td>
                    <td className="whitespace-nowrap text-[13px] text-muted">
                      {formatDateTime(u.first_seen_at)}
                    </td>
                    <td className="whitespace-nowrap text-[13px] text-muted">
                      {relativeTime(u.last_seen_at)}
                    </td>
                    <td className="text-right">
                      <Badge tone={u.status === "active" ? "success" : "warn"}>{u.status}</Badge>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section></>
  );
}
