import type { Metadata } from "next";
import { Code, Note } from "@/components/docs/DocsShell";

export const metadata: Metadata = { title: "Authentication" };

export default function AuthDocs() {
  return (
    <>
      <h1>Authentication</h1>
      <p>
        Four ways a principal can prove it has rights on a key. All four end the same way — the
        nodes bind a verified identity to an ephemeral session key, and every later request is
        signed with it.
      </p>

      <h2>Zero-knowledge proof of OAuth</h2>
      <p>
        The production route for end users. The client obtains an OAuth ID token and proves, inside
        a Noir circuit, that a token signed by a trusted RSA key commits to a given session public
        key. The token itself never reaches the network.
      </p>
      <p>
        The node verifies the proof against the circuit&rsquo;s verification key, which is compiled
        into the binary rather than configured, and checks the RSA modulus against its cached JWKS
        for that issuer. Where the group registers an issuer with a non-empty client allowlist, the
        token&rsquo;s <code>azp</code> — falling back to <code>aud</code> — must appear in it.
      </p>
      <Note tone="warn" title="An empty allowlist and a list containing an empty string are opposites">
        An empty array means <em>any client from this issuer</em>. An array containing an empty
        string matches nothing and rejects every client. It is an easy typo to make and a confusing
        one to debug.
      </Note>

      <h2>Authorization key certificate</h2>
      <p>
        The route your own backend uses. You hold a secp256k1 key whose public half is registered on
        the group contract, and you issue short-lived certificates binding an identity to a session
        key.
      </p>
      <Code title="certificate">{`signature = ECDSA(
  sha256(identity : group_id : session_pub_hex : expiry_be64)
)`}</Code>
      <p>
        This is the credential the console asks you to generate on the Keys screen. The private half
        is created in your browser and shown once — the platform stores only the public half, which
        is why the platform cannot authorize signing on your group even if it wanted to.
      </p>

      <h2>Delegation token</h2>
      <p>
        For autonomous agents. A JWT signed by a parent key, granting its holder access to one
        scoped sub-key for a bounded period, without any OAuth session. See{" "}
        <a href="/docs/session-signers">session signers</a>.
      </p>

      <h2>On-chain resolver (SIWE)</h2>
      <p>
        For identity that already lives on a chain. The client signs an ERC-4361 message; the node
        recovers the address and calls a resolver contract the group is bound to, reading at a
        block the client pinned so every node sees identical state.
      </p>
      <p>
        The binding is timelocked, and deliberately so: a resolver can authorize addresses
        unilaterally, so its blast radius is larger than an issuer&rsquo;s.
      </p>

      <h2>Signing a request</h2>
      <p>
        Once a session exists, every keygen and sign request carries a signature over a canonical
        hash. Each participating node recomputes it independently, so the node that received the
        request cannot substitute a different payload on the way to the others.
      </p>
      <Code title="canonical request hash">{`SHA256(group_id : key_id : nonce : timestamp_be64 [: message_hash])`}</Code>
      <p>
        For structured payloads, <code>message_hash</code> is the hash the client computed
        locally — for EIP-712, <code>hashTypedData</code> of the typed data being signed. Every node
        recomputes it from the forwarded payload before accepting the session signature.
      </p>
    </>
  );
}
