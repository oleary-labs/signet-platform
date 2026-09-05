import Link from "next/link";
import type { Metadata } from "next";
import { Note } from "@/components/docs/DocsShell";

export const metadata: Metadata = {
  title: "Documentation",
  description: "How to build on Signet: authentication, keys, scoped signing, and the platform API.",
};

export default function DocsOverview() {
  return (
    <>
      <h1>Documentation</h1>
      <p>
        Signet gives your application embedded wallets whose signing authority is split across
        operators you choose. This section covers what that means in practice — how a user
        authenticates, how a key comes into existence, and how you constrain what it may sign.
      </p>

      <h2>Where things live</h2>
      <p>
        Three layers, and it is worth keeping them apart when something goes wrong, because they
        fail differently.
      </p>
      <ul>
        <li>
          <strong>The protocol.</strong> A network of nodes that jointly generate keys and produce
          signatures. Chain-agnostic — it emits a signature, and knows nothing about EVM, accounts,
          or bundlers.
        </li>
        <li>
          <strong>The account-abstraction bridge.</strong> How a threshold Schnorr signature reaches
          the EVM, since there is no precompile for it. Today that is ERC-4337. Assume this layer
          changes; nothing in your group configuration depends on it.
        </li>
        <li>
          <strong>This platform.</strong> Metadata, configuration, analytics, and billing. It reads
          the chain and caches what it reads. It cannot sign for you, and it cannot change your
          group — those are properties of the design, not policy choices.
        </li>
      </ul>

      <Note title="What the platform cannot do">
        It holds no key your operators would accept, so it cannot authorize a signature. It never
        writes to the chain, so it cannot alter your group&rsquo;s membership, threshold, or
        issuers. Anything in the console that changes on-chain state is a transaction from your own
        account, built in your browser. If this platform disappeared tomorrow, your group would
        keep serving your users.
      </Note>

      <h2>Start here</h2>
      <ul>
        <li>
          <Link href="/docs/quickstart">Quickstart</Link> — from an empty project to a signature.
        </li>
        <li>
          <Link href="/docs/concepts">Core concepts</Link> — groups, thresholds, sessions, scopes.
        </li>
        <li>
          <Link href="/docs/auth">Authentication</Link> — the four routes a principal can prove
          itself by.
        </li>
      </ul>

      <h2>The repositories</h2>
      <p>Everything is open source, and the parts fit together like this:</p>
      <ul>
        <li>
          <a href="https://github.com/oleary-labs/signet-protocol">signet-protocol</a> — the node,
          the KMS, and the group contracts.
        </li>
        <li>
          <a href="https://github.com/oleary-labs/signet-sdk">signet-sdk</a> — the TypeScript client
          you will actually import.
        </li>
        <li>
          <a href="https://github.com/oleary-labs/signet-circuits">signet-circuits</a> — the Noir
          circuit that proves an OAuth credential without revealing it.
        </li>
        <li>
          <a href="https://github.com/oleary-labs/signet-wallet">signet-wallet</a> — the smart
          account and its on-chain FROST verifier.
        </li>
        <li>
          <a href="https://github.com/oleary-labs/signet-min-bundler">signet-min-bundler</a> — a
          minimal ERC-4337 bundler, and the server-side prover.
        </li>
      </ul>
    </>
  );
}
