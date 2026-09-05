export interface NavItem {
  href: string;
  label: string;
  /** Exact-match only, for hrefs that are prefixes of their siblings. */
  exact?: boolean;
}

export interface NavGroup {
  title?: string;
  items: NavItem[];
}

/**
 * The console's navigation.
 *
 * The shape follows what a developer coming from an embedded-wallet product
 * already knows — users, wallets, analytics, then configuration — with one
 * section that has no equivalent there: the signing group. That is where the
 * operator set and threshold live, and it is the reason to be here at all, so
 * it sits above settings rather than buried in them.
 */
export function appNav(appId: string): NavGroup[] {
  const base = `/apps/${appId}`;
  return [
    {
      items: [
        { href: base, label: "Overview", exact: true },
        { href: `${base}/users`, label: "Users" },
        { href: `${base}/wallets`, label: "Wallets" },
        { href: `${base}/analytics`, label: "Analytics" },
        { href: `${base}/logs`, label: "Activity" },
      ],
    },
    {
      title: "Signing group",
      items: [
        { href: `${base}/group`, label: "Operators & threshold" },
        { href: `${base}/credentials`, label: "Keys & credentials" },
      ],
    },
    {
      title: "Configuration",
      items: [
        { href: `${base}/configuration/login-methods`, label: "Login methods" },
        { href: `${base}/configuration/branding`, label: "Branding" },
        { href: `${base}/configuration/wallets`, label: "Embedded wallets" },
        { href: `${base}/configuration/smart-accounts`, label: "Smart accounts" },
        { href: `${base}/configuration/session-signers`, label: "Session signers" },
        { href: `${base}/configuration/policies`, label: "Policies" },
        { href: `${base}/configuration/funding`, label: "Funding" },
      ],
    },
    {
      title: "Settings",
      items: [
        { href: `${base}/settings`, label: "Basics", exact: true },
        { href: `${base}/settings/domains`, label: "Domains" },
        { href: `${base}/settings/webhooks`, label: "Webhooks" },
      ],
    },
  ];
}

export function orgNav(orgId: string): NavGroup[] {
  const base = `/org/${orgId}`;
  return [
    {
      title: "Organization",
      items: [
        { href: `${base}/members`, label: "Members" },
        { href: `${base}/billing`, label: "Billing" },
        { href: `${base}/settings`, label: "Settings" },
      ],
    },
  ];
}

export function isActive(pathname: string, item: NavItem): boolean {
  if (item.exact) return pathname === item.href;
  return pathname === item.href || pathname.startsWith(`${item.href}/`);
}
