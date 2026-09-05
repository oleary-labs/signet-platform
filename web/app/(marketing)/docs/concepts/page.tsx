import type { Metadata } from "next";
import { Note } from "@/components/docs/DocsShell";

export const metadata: Metadata = { title: "Core concepts" };

export default function Concepts() {
  return (
    <>
      <h1>Core concepts</h1>
      <p>Six ideas. Everything else in these docs is built from them.</p>

      <h2>Group</h2>
      <p>
        A set of node operators that jointly hold keys for one application, plus the configuration
        that governs them — the threshold, the trusted issuers, the authorization keys, the removal
        timelock. A group is a contract on-chain, and it is the source of truth: every node reads
        its own membership and policy from the chain rather than from anything an operator told it.
      </p>

      <h2>Threshold</h2>
      <p>
        The minimum number of operators that must contribute for a signature to exist. In a 3-of-5
        group, any three suffice, two can be offline, and an attacker needs three simultaneously —
        in three different companies, in three different failure domains. The threshold and the
        group size are yours to set, and yours to change.
      </p>
      <Note title="Threshold means the same thing everywhere here">
        In this documentation, in the console, and in the contracts, &ldquo;threshold&rdquo; is the
        minimum number of honest signers required — the standard FROST meaning. Quorum is the same
        number.
      </Note>

      <h2>Key</h2>
      <p>
        A public key whose private counterpart has never existed anywhere. It was constructed in
        pieces by a distributed key generation round, and each operator holds one share. Keys are
        identified by a key ID scoped to the group; a key ID under a different curve is a different
        key.
      </p>

      <h2>Session</h2>
      <p>
        Before a principal can ask for a signature it opens a session: it proves an identity once,
        and the nodes bind that identity to an ephemeral public key. Every subsequent request is
        signed by the matching private key. This is what keeps a replayable credential — a JWT, a
        password — from being sent to the network on every call.
      </p>

      <h2>Scope</h2>
      <p>
        An optional constraint stored with a key that restricts what it may sign. A scoped key
        refuses raw hashes; the caller must present a structured payload, and every operator
        independently re-derives the constraint and checks it before contributing a share.
      </p>
      <ul>
        <li>
          <strong>EIP-712 domain and type</strong> — chain, verifying contract, and the exact typed
          data method. A key that can sign <code>TransferWithAuthorization</code> on one contract
          cannot sign <code>permit</code> on the same one.
        </li>
        <li>
          <strong>EVM UserOperation</strong> — bound to one entry point, chain, and account.
        </li>
        <li>
          <strong>Solana transaction</strong> — bound to one wallet authority.
        </li>
      </ul>

      <h2>Reshare</h2>
      <p>
        Regenerating every operator&rsquo;s share of every key, without changing any public key or
        address. This is what makes the operator set genuinely mutable: you can add, remove, or
        replace operators and refresh the shares afterwards, and nothing your users hold has to
        migrate. Old shares stop being useful, which also makes reshare a reasonable scheduled
        hygiene operation.
      </p>

      <h2>How they compose</h2>
      <p>
        A user signs in and opens a <em>session</em>. Your app asks for a <em>key</em>, which the{" "}
        <em>group</em> generates in shares. To sign, a <em>threshold</em> of operators each verify
        the session and the key&rsquo;s <em>scope</em>, then contribute. If an operator has to go,
        you remove it and <em>reshare</em> — and your users never notice.
      </p>
    </>
  );
}
