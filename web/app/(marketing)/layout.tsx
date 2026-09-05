import ScrollFX from "@/components/ScrollFX";
import { SiteFooter } from "@/components/SiteFooter";
import { SiteHeader } from "@/components/SiteHeader";

export default function MarketingLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <SiteHeader />
      <ScrollFX />
      <main className="pt-[var(--header-h)]">{children}</main>
      <SiteFooter />
    </>
  );
}
