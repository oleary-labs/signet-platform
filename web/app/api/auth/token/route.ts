import type { NextRequest } from "next/server";

/**
 * Server-side token exchange for the Google OAuth PKCE flow.
 *
 * The browser sends the authorization code and PKCE verifier; this route adds
 * the client secret, which must never reach the browser, and exchanges with
 * Google. It runs in the web app rather than the API because the secret belongs
 * to this OAuth client, not to the platform's data plane.
 */
export async function POST(request: NextRequest) {
  const { code, code_verifier, redirect_uri } = await request.json();

  const clientId = process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID;
  const clientSecret = process.env.GOOGLE_CLIENT_SECRET;
  if (!clientId || !clientSecret) {
    return Response.json(
      {
        error:
          "Google sign-in is not configured. Set NEXT_PUBLIC_GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET.",
      },
      { status: 500 },
    );
  }

  const res = await fetch("https://oauth2.googleapis.com/token", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      client_id: clientId,
      client_secret: clientSecret,
      code,
      code_verifier,
      grant_type: "authorization_code",
      redirect_uri,
    }),
  });

  const tokens = await res.json();
  if (tokens.error) {
    return Response.json(
      { error: tokens.error, error_description: tokens.error_description },
      { status: 400 },
    );
  }

  // Only the ID token is returned. The access token would let the browser call
  // Google on the user's behalf, which this flow has no need for.
  return Response.json({ id_token: tokens.id_token });
}
