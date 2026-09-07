/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // `npm run build` writes to the canonical .next, because that is the only
  // output directory Vercel's Next.js builder accepts and the one the
  // Dockerfile copies. Building while `boot.sh` is up would then clobber the
  // dev server's chunks — "Cannot find module './5873.js'" with no obvious
  // cause — so `npm run build:local` sets NEXT_DIST_DIR to build somewhere
  // else. Deployments must never set it.
  distDir: process.env.NEXT_DIST_DIR || ".next",
  // Node and operator logos are developer-supplied URLs on arbitrary hosts, so
  // they are rendered as plain <img> rather than through next/image — there is
  // no fixed allowlist to configure.
  images: { unoptimized: true },
};

export default nextConfig;
