import type { Metadata } from "next";
import { Code, Note } from "@/components/docs/DocsShell";

export const metadata: Metadata = { title: "Session signers" };

export default function SessionSignersDocs() {
  return (
    <>
      <h1>Session signers</h1>
      <p>
        An agent that pays for API calls cannot hold the user&rsquo;s OAuth session — sessions
        expire, and the user is not there to renew one. A session signer, which the protocol calls
        a delegation token, is a bounded grant that lets an agent use one scoped sub-key on its own.
      </p>

      <h2>The shape of the grant</h2>
      <p>
        A JWT signed by the parent key, threshold-signed by the group. It names the sub-key it
        grants access to, the parent key that signed it, and an expiry.
      </p>
      <Code title="claims">{`{
  "iss": "0xGroupAddress",
  "sub": "oauth:https://accounts.google.com:1234:a1b2c3d4",  // the sub-key
  "kid": "oauth:https://accounts.google.com:1234",           // the parent
  "scheme": "ecdsa_secp256k1",
  "exp": 1735689600,
  "parent_key_pub": "02…"
}`}</Code>

      <h2>Minting one</h2>
      <Code title="delegate.ts">{`import { requestDelegation } from "@oleary-labs/signet-sdk/delegate";

const delegation = await requestDelegation(
  NODE_URL,
  "/api/node/proxy",
  GROUP_ADDRESS,
  scopeSuffix,        // the sub-key
  parentKeyId,
  "ecdsa_secp256k1",
  30 * 24 * 60 * 60,  // 30 days
  sessionKeypair,
  claims,
);`}</Code>

      <h2>Using one</h2>
      <Code title="agent.ts">{`import { authenticateWithDelegation } from "@oleary-labs/signet-sdk/delegate";

await authenticateWithDelegation(NODE_URL, delegation.token, sessionPub);
// The agent now signs with the sub-key, within its scope, with no user present.`}</Code>

      <Note tone="warn" title="Revocation is not a database row">
        A delegation token is verified by the nodes against the parent key. Nothing on this
        platform — and nothing in your own database — can make an issued token stop verifying.{" "}
        <strong>Disable the sub-key on the nodes.</strong> That is enforced by every operator, and
        it is the only step that actually cuts the agent off. The console pairs the two actions and
        says so.
      </Note>

      <h2>Designing the grant</h2>
      <ul>
        <li>
          <strong>Always scope the sub-key.</strong> Delegating an unscoped key hands an agent
          unlimited signing authority for the token&rsquo;s lifetime.
        </li>
        <li>
          <strong>One key per purpose.</strong> Separate sub-keys mean you can revoke one agent
          without touching the others.
        </li>
        <li>
          <strong>Fund deliberately.</strong> A scoped payment key can spend whatever its address
          holds — the balance <em>is</em> the limit, so treat funding as the budget.
        </li>
        <li>
          <strong>Short lifetimes.</strong> Thirty days is a reasonable default. A year is not.
        </li>
      </ul>
    </>
  );
}
