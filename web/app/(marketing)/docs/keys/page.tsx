import type { Metadata } from "next";
import { Code, Note } from "@/components/docs/DocsShell";

export const metadata: Metadata = { title: "Keys and signing" };

export default function KeysDocs() {
  return (
    <>
      <h1>Keys and signing</h1>

      <h2>Three schemes</h2>
      <p>
        One key generation and coordination stack, three signing schemes, selected per request with
        a <code>curve</code> parameter.
      </p>
      <div className="table-scroll not-prose my-5 overflow-x-auto">
        <table className="table">
          <thead>
            <tr>
              <th>Curve</th>
              <th>Algorithm</th>
              <th>Verified by</th>
              <th>Signature</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td className="mono">frost_secp256k1</td>
              <td>FROST Schnorr (RFC 9591)</td>
              <td>A verifier library, via a smart account</td>
              <td>65 bytes — R.x ‖ z ‖ v</td>
            </tr>
            <tr>
              <td className="mono">ecdsa_secp256k1</td>
              <td>Threshold ECDSA</td>
              <td className="mono">ecrecover</td>
              <td>65 bytes — r ‖ s ‖ v</td>
            </tr>
            <tr>
              <td className="mono">frost_ed25519</td>
              <td>FROST Schnorr over Ed25519</td>
              <td>Native Ed25519 verifiers</td>
              <td>64 bytes</td>
            </tr>
          </tbody>
        </table>
      </div>

      <Note title="The curve is part of a key's identity">
        The same key ID under a different curve is a different key, with different shares and a
        different address. Always pass <code>curve</code> explicitly; the node never guesses.
      </Note>

      <h2>Which one to pick</h2>
      <p>
        For anything that needs to look like an ordinary Ethereum signature — EIP-712, EIP-3009,
        anything a contract verifies with <code>ecrecover</code> — use{" "}
        <code>ecdsa_secp256k1</code>. For a smart account whose validator is Signet&rsquo;s own, use{" "}
        <code>frost_secp256k1</code>. For Solana and other chains with native Ed25519, use{" "}
        <code>frost_ed25519</code>.
      </p>

      <h2>Key lifecycle</h2>
      <ul>
        <li>
          <strong>Generate</strong> — a distributed round; every operator ends with a share.
        </li>
        <li>
          <strong>Disable</strong> — the kill switch. The key refuses to sign and refuses to mint
          delegations. Enforced by every operator, not by any one of them.
        </li>
        <li>
          <strong>Enable</strong> — reverses a disable.
        </li>
        <li>
          <strong>Delete</strong> — removes the shares. There is no undo, and no recovery.
        </li>
      </ul>
      <Code title="disable a key">{`await fetch(\`\${NODE_URL}/v1/keys/disable\`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    group_id: GROUP_ADDRESS,
    key_id: KEY_ID,
    curve: "ecdsa_secp256k1",
    ...signedRequest,
  }),
});`}</Code>

      <h2>Listing what your group holds</h2>
      <p>
        <code>POST /admin/keys</code> returns the inventory for a group. It requires a signature
        from an authorization key the group trusts, which is why the console asks you to sync it
        rather than fetching it on your behalf — this platform deliberately holds no credential your
        operators would accept.
      </p>

      <h2>Reshare</h2>
      <p>
        Calling <code>requestReshare</code> on your group contract emits an event; the nodes elect a
        leader and run the protocol. Public keys and addresses are unchanged throughout, so nothing
        your users hold has to move. Worth doing after an operator leaves, and worth scheduling if
        you want a stolen share to expire on its own.
      </p>
    </>
  );
}
