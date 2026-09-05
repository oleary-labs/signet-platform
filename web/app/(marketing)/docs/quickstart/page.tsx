import Link from "next/link";
import type { Metadata } from "next";
import { Code, Note } from "@/components/docs/DocsShell";

export const metadata: Metadata = { title: "Quickstart" };

export default function Quickstart() {
  return (
    <>
      <h1>Quickstart</h1>
      <p>
        From nothing to a signature. This assumes you have created an app in the console and
        deployed a signing group — if you have not, do that first; it takes a few minutes.
      </p>

      <h2>1. Install the SDK</h2>
      <Code title="shell">{`npm install @oleary-labs/signet-sdk viem`}</Code>
      <p>
        Client-side zero-knowledge proving additionally needs{" "}
        <code>@noir-lang/noir_js</code>, <code>@aztec/bb.js</code>, and{" "}
        <code>@oleary-labs/signet-circuits</code>. They are optional peer dependencies — skip them
        if you prove server-side.
      </p>

      <h2>2. Authenticate the user</h2>
      <p>
        The user signs in with OAuth. Their token never reaches the network: a zero-knowledge proof
        establishes that they hold a valid credential from an issuer your group trusts, bound to an
        ephemeral session key.
      </p>
      <Code title="signIn.ts">{`import { startGoogleOAuth, decodeIdToken, handleOAuthCallback } from "@oleary-labs/signet-sdk/oauth";
import { generateSessionKeypair } from "@oleary-labs/signet-sdk/session";
import { generateJWTProof, getJWTModulusBytes } from "@oleary-labs/signet-sdk/proof";
import { authenticateWithBootstrap } from "@oleary-labs/signet-sdk/bootstrap";

// Kick the user to Google.
await startGoogleOAuth({ clientId: process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID! });

// …and back on your callback route:
const jwt = await handleOAuthCallback("/api/auth/token");
const claims = decodeIdToken(jwt);
const keypair = await generateSessionKeypair();

// The slow step — a few seconds, once per sign-in.
const { proof } = await generateJWTProof(jwt, keypair.publicKeyHex);
const modulus = await getJWTModulusBytes(jwt);

await authenticateWithBootstrap(
  { groupId: GROUP_ADDRESS, nodeUrls: NODE_URLS },
  proof,
  keypair.publicKeyHex,
  claims,
  modulus,
);`}</Code>

      <Note title="Why the proof exists">
        Forwarding the raw token would put a replayable credential on every node in your group. The
        proof establishes the same fact — this person holds a valid token from a trusted issuer —
        while leaving nothing behind that anyone could reuse.
      </Note>

      <h2>3. Create the user&rsquo;s key</h2>
      <p>
        Key generation is a distributed protocol: every operator ends up with one share, and no
        machine anywhere ever holds the whole key.
      </p>
      <Code title="keygen.ts">{`import { keygen } from "@oleary-labs/signet-sdk/keygen";

const key = await keygen(
  { groupId: GROUP_ADDRESS, nodeUrls: NODE_URLS },
  keypair,
  claims,
);

console.log(key.ethereumAddress); // the user's address
console.log(key.alreadyExisted);  // true on every sign-in after the first`}</Code>

      <h2>4. Sign something</h2>
      <Code title="sign.ts">{`import { signSignRequest } from "@oleary-labs/signet-sdk/request";

const signed = await signSignRequest(keypair, claims, GROUP_ADDRESS, messageHash);

const res = await fetch(\`\${NODE_URLS[0]}/v1/sign\`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ ...signed, curve: "frost_secp256k1" }),
});

const { ethereum_signature } = await res.json();`}</Code>
      <p>
        You contact one node. It coordinates the round with the rest of the group over their own
        peer-to-peer mesh, and returns when a quorum has contributed. Every participant
        independently re-checks your session signature before it takes part — the node you happened
        to call cannot vouch for you to the others.
      </p>

      <h2>5. Constrain what the key may sign</h2>
      <p>
        A key with no scope will sign any hash you hand it. For anything an agent or a background
        job touches, mint a <Link href="/docs/scoped-keys">scoped sub-key</Link> instead — bound to
        one chain, one contract, and one message type, enforced by every operator.
      </p>

      <h2>Running it all locally</h2>
      <Code title="shell">{`# Terminal 1 — Anvil, contracts, three nodes, a bootstrap group
cd signet-protocol && devnet/start.sh

# Terminal 2 — the bundler and paymaster
cd signet-min-bundler && scripts/devnet-setup.sh && ./bundler --config bundler.toml

# Terminal 3 — this platform
cd platform/backend && go run ./cmd/server
cd platform/web && npm run dev`}</Code>
    </>
  );
}
