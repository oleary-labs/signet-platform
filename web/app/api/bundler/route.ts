import type { NextRequest } from "next/server";

/**
 * Server-side proxy to the bundler.
 *
 * The browser cannot call it directly (no CORS headers), and the server-side
 * prover endpoint takes an API key that must not ship to the client. Set
 * `x-bundler-path` to reach a sub-path such as /v1/prove; the default is the
 * JSON-RPC root.
 */
export async function POST(request: NextRequest) {
  const bundlerUrl = process.env.NEXT_PUBLIC_BUNDLER_URL;
  if (!bundlerUrl) {
    return Response.json({ error: "NEXT_PUBLIC_BUNDLER_URL is not set" }, { status: 500 });
  }

  const body = await request.text();
  const path = request.headers.get("x-bundler-path") ?? "";
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const apiKey = process.env.PROVER_API_KEY;
  if (apiKey && path === "/v1/prove") headers["X-API-Key"] = apiKey;

  try {
    const res = await fetch(`${bundlerUrl.replace(/\/$/, "")}${path}`, {
      method: "POST",
      headers,
      body,
    });
    return new Response(await res.text(), {
      status: res.status,
      headers: { "Content-Type": "application/json" },
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return Response.json(
      { jsonrpc: "2.0", id: 1, error: { code: -32000, message } },
      { status: 502 },
    );
  }
}
