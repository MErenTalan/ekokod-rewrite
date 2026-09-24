'use client';

import { Leaf, Menu } from 'lucide-react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { LanguageSwitcher } from '@/components/shell/language-switcher';
import { ThemeToggle } from '@/components/shell/theme-toggle';
import { Button } from '@/components/ui/button';
import { Drawer } from '@/components/ui/drawer';
import { IconButton } from '@/components/ui/icon-button';
import { cn } from '@/lib/cn';

import { isCurrent, siteNav } from './site-nav';

export type SiteHeaderProps = { pricing: boolean; signedIn: boolean };

/** Responsive site header: inline links from 1280 px, a drawer below (01 §7.19). */
export function SiteHeader({ pricing, signedIn }: SiteHeaderProps) {
  const t = useTranslations('site');
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  const links = siteNav(pricing);
  const account = signedIn ? { href: '/ekorm', label: t('nav.platform') } : { href: '/auth/login', label: t('nav.login') };

  return (
    <header className="sticky top-0 z-40 border-b border-border bg-surface/95 backdrop-blur supports-[backdrop-filter]:bg-surface/80">
      <div className="mx-auto flex h-16 max-w-7xl items-center gap-4 px-4 sm:px-6">
        <Link href="/" className="flex shrink-0 items-center gap-1.5 rounded-md pointer-coarse:min-h-11">
          <Leaf aria-hidden className="size-6 stroke-brand" />
          <span className="font-heading text-lg font-semibold text-foreground">{t('brand')}</span>
        </Link>
        <nav aria-label={t('nav.label')} className="hidden flex-1 justify-center xl:flex">
          <ul className="flex items-center gap-1">
            {links.map((l) => {
              const current = isCurrent(pathname, l.href);
              return (
                <li key={l.key}>
                  <Link
                    href={l.href}
                    aria-current={current ? 'page' : undefined}
                    className={cn(
                      'inline-flex h-9 items-center rounded-md px-3 type-body font-medium text-foreground-muted transition-colors hover:bg-surface-sunken hover:text-foreground',
                      current && 'bg-surface-sunken text-foreground',
                    )}
                  >
                    {t(`nav.${l.key}`)}
                  </Link>
                </li>
              );
            })}
          </ul>
        </nav>
        <div className="ms-auto flex items-center gap-1 xl:ms-0">
          <LanguageSwitcher />
          <ThemeToggle />
          <Button asChild variant="ghost" className="hidden sm:inline-flex">
            <Link href={account.href}>{account.label}</Link>
          </Button>
          <Button asChild className="hidden md:inline-flex">
            <Link href="/request-demo">{t('nav.requestDemo')}</Link>
          </Button>
          <span className="xl:hidden">
            <IconButton label={t('nav.openMenu')} icon={Menu} onClick={() => setOpen(true)} />
          </span>
        </div>
      </div>
      <Drawer side="end" open={open} onOpenChange={setOpen} title={t('nav.menu')}>
        <nav aria-label={t('nav.label')} className="px-4 pb-4">
          <ul className="flex flex-col gap-1">
            {[{ key: 'home' as const, href: '/' }, ...links].map((l) => (
              <li key={l.key}>
                <Link
                  href={l.href}
                  onClick={() => setOpen(false)}
                  aria-current={(l.href === '/' ? pathname === '/' : isCurrent(pathname, l.href)) ? 'page' : undefined}
                  className="flex min-h-11 items-center rounded-md px-3 type-body-lg text-foreground hover:bg-surface-sunken aria-[current=page]:bg-surface-sunken aria-[current=page]:font-semibold"
                >
                  {t(`nav.${l.key}`)}
                </Link>
              </li>
            ))}
          </ul>
          <div className="mt-4 flex flex-col gap-2 border-t border-border pt-4">
            <Button asChild size="lg">
              <Link href="/request-demo" onClick={() => setOpen(false)}>{t('nav.requestDemo')}</Link>
            </Button>
            <Button asChild variant="secondary" size="lg">
              <Link href={account.href} onClick={() => setOpen(false)}>{account.label}</Link>
            </Button>
          </div>
        </nav>
      </Drawer>
    </header>
  );
}
