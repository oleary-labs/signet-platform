import type { Metadata } from "next";
import { Code, Note } from "@/components/docs/DocsShell";

export const metadata: Metadata = { title: "Scoped sub-keys" };

export default function ScopedKeysDocs() {
  return (
    <>
      <h1>Scoped sub-keys</h1>
      <p>
        A key with no scope will sign any 32 bytes you hand it. That is fine for a key a person
        drives through a confirmation dialog, and unacceptable for one an agent holds. A scope binds
        a key to a specific thing it is allowed to say.
      </p>

      <h2>What a scope binds</h2>
      <p>
        Scope bytes are <code>[1-byte scheme][scheme-specific bytes]</code>. Three schemes exist
        today.
      </p>
      <div className="table-scroll not-prose my-5 overflow-x-auto">
        <table className="table">
          <thead>
            <tr>
              <th>Scheme</th>
              <th>Byte</th>
              <th>Binds</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>Unscoped</td>
              <td className="mono">0x00</td>
              <td>Nothing — signs any hash</td>
            </tr>
            <tr>
              <td>EVM UserOperation</td>
              <td className="mono">0x01</td>
              <td>entryPoint · chainId · sender</td>
            </tr>
            <tr>
              <td>Solana transaction</td>
              <td className="mono">0x02</td>
              <td>wallet authority</td>
            </tr>
            <tr>
              <td>EIP-712</td>
              <td className="mono">0x03</td>
              <td>chainId · verifyingContract · typeHash</td>
            </tr>
          </tbody>
        </table>
      </div>

      <Note title="The type hash matters as much as the contract">
        Binding only the domain would let a key authorized for{" "}
        <code>TransferWithAuthorization</code> also sign <code>permit</code> on the same contract —
        which is an unlimited approval wearing a payment&rsquo;s clothes. The scope commits to the
        primary type&rsquo;s full field layout, so the two are different keys.
      </Note>

      <h2>Deriving the key ID</h2>
      <p>
        A sub-key&rsquo;s suffix comes from its scope, not from a name you choose:
      </p>
      <Code>{`key_suffix = hex(sha256(scope)[:8])
key_id     = oauth:<iss>:<sub>:<scope_hash>`}</Code>
      <p>
        The same user with the same scope always resolves to the same key, so minting is idempotent
        and there is no way to accumulate duplicate keys for one purpose.
      </p>

      <h2>Creating one</h2>
      <Code title="scopedKey.ts">{`import { buildEIP712Scope } from "@oleary-labs/signet-sdk/scopedSign";
import { keygen } from "@oleary-labs/signet-sdk/keygen";

// USDC on Base, EIP-3009 transfers only.
const scope = buildEIP712Scope({
  chainId: 8453,
  verifyingContract: "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
  primaryType: "TransferWithAuthorization",
  types: EIP3009_TYPES,
});

const key = await keygen(
  { groupId: GROUP_ADDRESS, nodeUrls: NODE_URLS },
  keypair,
  claims,
  undefined,
  undefined,
  "ecdsa_secp256k1",
  scope,
);`}</Code>

      <h2>Signing with one</h2>
      <p>
        A scoped key refuses a raw hash. You send the structured payload; every node parses it,
        re-derives the domain and type hash, checks them against the stored scope, and computes the
        signing hash itself.
      </p>
      <Code title="sign.ts">{`import { signTypedData } from "@oleary-labs/signet-sdk/scopedSign";

const signature = await signTypedData({
  nodeUrl: NODE_URLS[0],
  groupId: GROUP_ADDRESS,
  keySuffix: key.keyId,
  curve: "ecdsa_secp256k1",
  typedData,
  sessionKeypair: keypair,
  claims,
});`}</Code>
      <p>
        That independent re-derivation is the whole guarantee. A malicious coordinating node cannot
        substitute a different payload, because the other operators never trusted its hash in the
        first place.
      </p>

      <h2>Where scopes stop</h2>
      <p>
        A scope constrains <em>what</em> may be signed — never <em>how much</em> or{" "}
        <em>how often</em>. A key scoped to USDC transfers on Base can transfer the whole balance,
        as many times as it likes. Value and rate limits belong in the smart account, where the
        chain enforces them against settled state at execution time.
      </p>
    </>
  );
}
