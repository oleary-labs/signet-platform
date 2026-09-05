import type { Metadata } from "next";
import { Code, Note } from "@/components/docs/DocsShell";

export const metadata: Metadata = { title: "Platform API" };

const ENDPOINTS: { group: string; rows: [string, string, string][] }[] = [
  {
    group: "Public",
    rows: [
      ["GET", "/v1/marketplace/nodes", "The operator directory, with liveness."],
      ["GET", "/v1/marketplace/nodes/{address}", "One operator."],
      ["GET", "/v1/status", "Network status: chain reachability, node and group counts."],
      ["GET", "/v1/config", "The protocol wiring this deployment is pointed at."],
      ["GET", "/v1/webhooks/events", "Every event a webhook can subscribe to."],
    ],
  },
  {
    group: "Session",
    rows: [
      ["POST", "/v1/auth/challenge", "Issue a single-use sign-in nonce."],
      ["POST", "/v1/auth/verify", "Verify the signed challenge and open a session."],
      ["POST", "/v1/auth/logout", "Clear the session cookie."],
      ["GET", "/v1/me", "The signed-in developer, their orgs, and the network config."],
      ["GET", "/v1/me/apps", "Every app across every org you belong to."],
    ],
  },
  {
    group: "Organizations",
    rows: [
      ["GET", "/v1/orgs", "Your organizations."],
      ["POST", "/v1/orgs", "Create one."],
      ["GET", "/v1/orgs/{orgID}/members", "Members and roles."],
      ["POST", "/v1/orgs/{orgID}/invites", "Create an invitation; the token is returned once."],
      ["GET", "/v1/orgs/{orgID}/billing", "Balance, rate, and the projected cost of current usage."],
      ["GET", "/v1/orgs/{orgID}/audit", "Organization activity."],
    ],
  },
  {
    group: "Apps",
    rows: [
      ["GET", "/v1/apps/{appID}", "One app."],
      ["PATCH", "/v1/apps/{appID}", "Rename, re-describe, change environment."],
      ["GET", "/v1/apps/{appID}/settings", "Every configuration section."],
      ["PUT", "/v1/apps/{appID}/settings/{section}", "Replace one section."],
      ["GET", "/v1/apps/{appID}/group", "Cached membership plus a live read of the contract."],
      ["POST", "/v1/apps/{appID}/group/attach", "Bind a deployed group, after verifying you manage it."],
      ["POST", "/v1/apps/{appID}/group/sync", "Re-read the contract now."],
      ["GET", "/v1/apps/{appID}/keys", "Cached key inventory and summary counts."],
      ["POST", "/v1/apps/{appID}/keys/sync", "Cache an inventory you fetched from your nodes."],
      ["GET", "/v1/apps/{appID}/users", "End users, by identity hash."],
      ["GET", "/v1/apps/{appID}/usage", "Daily metered activity for a date range."],
      ["GET", "/v1/apps/{appID}/audit", "App activity."],
    ],
  },
  {
    group: "Metering",
    rows: [["POST", "/v1/ingest/usage", "Node-fleet ingest, authenticated by X-Ingest-Key."]],
  },
];

export default function ApiDocs() {
  return (
    <>
      <h1>Platform API</h1>
      <p>
        The console is a client of this API and uses nothing private. Everything it does, you can do
        from a script.
      </p>

      <h2>Authentication</h2>
      <p>
        Sign in and you get a session token, sent as a bearer credential or an HttpOnly cookie. For
        automation, create an application secret on the app&rsquo;s Keys screen and send it as{" "}
        <code>X-App-Secret</code>.
      </p>
      <Code title="shell">{`curl https://api.signet.dev/v1/me \\
  -H "Authorization: Bearer $SIGNET_SESSION"`}</Code>

      <Note title="What this API cannot do">
        It cannot sign, and it cannot write to a chain. Group membership, thresholds, issuers, and
        authorization keys are changed by transactions from your own account — the platform reads
        them and caches them, and every response that reflects chain state carries the time it was
        last read.
      </Note>

      <h2>Endpoints</h2>
      {ENDPOINTS.map((g) => (
        <div key={g.group}>
          <h3>{g.group}</h3>
          <div className="table-scroll not-prose my-4 overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Method</th>
                  <th>Path</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                {g.rows.map(([method, path, description]) => (
                  <tr key={path + method}>
                    <td className="mono whitespace-nowrap font-semibold">{method}</td>
                    <td className="mono whitespace-nowrap">{path}</td>
                    <td>{description}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ))}

      <h2>Errors</h2>
      <p>
        Every failure is a JSON object with an <code>error</code> field carrying a message meant for
        a person. A request for something you cannot see returns <code>404</code> rather than{" "}
        <code>403</code> — telling a stranger that an ID exists is itself a disclosure.
      </p>
      <Code title="response">{`{ "error": "that signing group is already attached to another app" }`}</Code>

      <h2>Reaching your nodes</h2>
      <p>
        <code>POST /v1/node/proxy</code> forwards a request to a node in the operator directory,
        because <code>signetd</code> sets no CORS headers and a browser cannot call it directly. The
        body is opaque to the platform: the signature inside it came from your key and is verified
        by the node, not here. Set <code>x-node-url</code> and <code>x-node-path</code>; only an
        explicit allowlist of node paths is forwarded.
      </p>
    </>
  );
}
