import type { Metadata, Viewport } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import { Providers } from "@/providers/Providers";
import "./globals.css";

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-sans",
  display: "swap",
});

const mono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
  display: "swap",
});

export const metadata: Metadata = {
  title: {
    default: "Signet — distributed key management for developers",
    template: "%s · Signet",
  },
  description:
    "Embedded wallet infrastructure with no single operator. Choose your own signing group, set your own threshold, and change either without downtime or key migration.",
  metadataBase: new URL(process.env.NEXT_PUBLIC_SITE_URL || "http://localhost:3000"),
  openGraph: {
    title: "Signet — distributed key management for developers",
    description:
      "Embedded wallet UX with sovereignty guarantees that do not depend on trusting any single entity, including us.",
    type: "website",
  },
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#faf9f7" },
    { media: "(prefers-color-scheme: dark)", color: "#0a1929" },
  ],
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        {/*
          Resolve the theme before first paint. Without this, a dark-mode
          console flashes the light palette on every navigation, which is
          worse than the small amount of inline script it takes to avoid.
        */}
        <script
          dangerouslySetInnerHTML={{
            __html: `(function(){try{var t=localStorage.getItem('signet_platform_theme')||'system';var d=t==='dark'||(t==='system'&&matchMedia('(prefers-color-scheme: dark)').matches);document.documentElement.classList.toggle('dark',d);}catch(e){}})();`,
          }}
        />
        {/*
          Arm the reveal system before first paint. The .fx hidden state keys
          off this class, so with scripting off nothing is ever hidden — the
          page renders complete without the animation instead of blank. Note
          that arming it is not the same as revealing it: an .fx element still
          needs a <ScrollFX /> above it to receive .in, so do not tag one
          outside a layout that mounts it.
        */}
        <script
          dangerouslySetInnerHTML={{
            __html: `document.documentElement.classList.add('fx-ready');`,
          }}
        />
      </head>
      <body className={`${inter.variable} ${mono.variable} font-sans antialiased`}>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
