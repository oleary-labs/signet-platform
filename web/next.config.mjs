/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // A production build writes to its own directory so it cannot corrupt a dev
  // server's chunks. Running `npm run build` while `boot.sh` is up is a normal
  // thing to do, and sharing `.next` makes the running console start throwing
  // "Cannot find module './5873.js'" with no obvious cause.
  distDir: process.env.NEXT_DIST_DIR || ".next",
  // Node and operator logos are developer-supplied URLs on arbitrary hosts, so
  // they are rendered as plain <img> rather than through next/image — there is
  // no fixed allowlist to configure.
  images: { unoptimized: true },
};

export default nextConfig;
