import type { Metadata } from "next";
import { Code, Note } from "@/components/docs/DocsShell";

export const metadata: Metadata = { title: "Webhooks" };

const EVENTS: [string, string][] = [
  ["app.deployed", "A signing group was attached to an app."],
  ["app.updated", "An app's details changed."],
  ["group.node_invited", "An operator was invited to a group."],
  ["group.node_joined", "An operator became active."],
  ["group.removal_queued", "A removal entered its timelock."],
  ["group.removal_executed", "An operator was removed."],
  ["group.reshare_requested", "A key refresh was requested."],
  ["issuer.added", "A login method was added."],
  ["issuer.removed", "A login method was removed."],
  ["auth_key.added", "An authorization key was recorded."],
  ["auth_key.revoked", "An authorization key was revoked."],
  ["key.created", "A user key was generated."],
  ["key.disabled", "A key was disabled."],
  ["key.enabled", "A key was re-enabled."],
  ["delegation.issued", "A session signer was minted."],
  ["delegation.revoked", "A session signer was revoked."],
  ["user.first_seen", "A user authenticated for the first time."],
  ["usage.threshold_reached", "Usage crossed a configured threshold."],
  ["billing.low_balance", "The billing balance fell below its floor."],
];

export default function WebhooksDocs() {
  return (
    <>
      <h1>Webhooks</h1>
      <p>
        Platform events posted to an endpoint of yours, so you learn about a membership change or a
        disabled key without polling for it.
      </p>

      <h2>The envelope</h2>
      <Code title="POST your-endpoint">{`{
  "id": "0b8b2b3e-…",
  "event": "key.disabled",
  "app_id": "6f2393b5-…",
  "created_at": "2026-08-31T18:12:04Z",
  "data": { "key_id": "oauth:…", "curve": "ecdsa_secp256k1" }
}`}</Code>

      <h2>Verifying a delivery</h2>
      <p>
        Two headers accompany every request. <code>Signet-Timestamp</code> is the send time, and{" "}
        <code>Signet-Signature</code> is an HMAC-SHA256 over the timestamp and the raw body
        together, keyed by the secret shown when you created the endpoint.
      </p>
      <Code title="verify.ts">{`import { createHmac, timingSafeEqual } from "node:crypto";

export function verify(secret: string, headers: Headers, rawBody: string) {
  const timestamp = headers.get("signet-timestamp") ?? "";
  const signature = headers.get("signet-signature") ?? "";

  // The timestamp is signed, but a genuine old delivery can still be
  // resent — so reject anything stale outright.
  if (!timestamp || Math.abs(Date.now() / 1000 - Number(timestamp)) > 300) return false;

  const mac = createHmac("sha256", secret);
  mac.update(timestamp);
  mac.update(".");
  mac.update(rawBody);            // the raw bytes, before any JSON parsing
  const expected = \`v1=\${mac.digest("hex")}\`;

  const a = Buffer.from(expected);
  const b = Buffer.from(signature);
  return a.length === b.length && timingSafeEqual(a, b);
}`}</Code>

      <Note tone="warn" title="Deliveries are not retried">
        A failed delivery is recorded and left alone. Treat webhooks as a fast path and reconcile
        against the API for anything you cannot afford to miss — an endpoint that was down for ten
        minutes will simply have missed those events.
      </Note>

      <h2>Events</h2>
      <div className="table-scroll not-prose my-5 overflow-x-auto">
        <table className="table">
          <thead>
            <tr>
              <th>Event</th>
              <th>Fires when</th>
            </tr>
          </thead>
          <tbody>
            {EVENTS.map(([name, when]) => (
              <tr key={name}>
                <td className="mono whitespace-nowrap">{name}</td>
                <td>{when}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p>
        An endpoint with no events selected receives everything, including events added later — the
        least surprising reading of &ldquo;I did not narrow it&rdquo;.
      </p>
    </>
  );
}
