import type { Metadata } from "next";
import { Code, Note } from "@/components/docs/DocsShell";

export const metadata: Metadata = { title: "Running a node" };

export default function OperatorsDocs() {
  return (
    <>
      <h1>Running a node</h1>
      <p>
        Operators are the supply side of this network. Applications choose you, pay you for
        availability, and can remove you — which is the whole accountability model, and why the
        requirements below are about uptime rather than hardware.
      </p>

      <h2>What you are agreeing to</h2>
      <ul>
        <li>
          Hold one share of every key in the groups you join. Never a whole key, and never enough
          to sign alone.
        </li>
        <li>
          Verify every request independently before contributing — the credential, the scope, and
          the payload hash. Never take another node&rsquo;s word for any of it.
        </li>
        <li>
          Stay reachable. Applications see your uptime, and an operator that is regularly missing
          gets replaced.
        </li>
      </ul>

      <h2>Requirements</h2>
      <p>
        Deliberately light: compute and uptime, no specialized hardware. A competent operator serves
        many applications from one instance and scales horizontally.
      </p>
      <ul>
        <li>Go 1.26+ and a Rust toolchain for the KMS.</li>
        <li>A reachable libp2p port and an HTTPS-terminated HTTP API.</li>
        <li>An Ethereum RPC endpoint for the chain your groups live on.</li>
        <li>Durable storage for key shares, and a backup procedure you have actually tested.</li>
      </ul>

      <Note tone="warn" title="Losing shares is not recoverable by anyone">
        If enough operators lose their shares at once, the key is gone — there is no vault, no
        escrow, and no support ticket that gets it back. That is the same property that means no
        one can seize it. Back up, and test the restore.
      </Note>

      <h2>Getting started</h2>
      <Code title="shell">{`git clone https://github.com/oleary-labs/signet-protocol
cd signet-protocol

go build ./cmd/signetd/
cd kms-tss && cargo build --release`}</Code>
      <p>
        Register on-chain with <code>SignetFactory.registerNode</code>, choosing whether you accept
        invitations automatically or require acceptance. Then send your operator details to the
        Signet team to appear in the marketplace with your own branding.
      </p>

      <h2>Before you take real traffic</h2>
      <ul>
        <li>
          Terminate TLS in front of the API. Session keys and signatures should not cross a network
          in plaintext.
        </li>
        <li>
          Rate-limit the endpoints. <code>/v1/auth</code> triggers proof verification, and{" "}
          <code>/v1/keygen</code> and <code>/v1/sign</code> consume protocol resources.
        </li>
        <li>
          Encrypt key material at rest, and set the node key passphrase before first start.
        </li>
        <li>Ship structured logs somewhere durable — you will need them during an incident.</li>
      </ul>
      <p>
        The protocol repository&rsquo;s <code>docs/PRODUCTION-GAPS.md</code> is the authoritative
        list of what is and is not production-ready. Read it before you operate for anyone
        else&rsquo;s users.
      </p>
    </>
  );
}
