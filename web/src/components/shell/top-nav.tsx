'use client';

import { ChevronDown } from 'lucide-react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { cn } from '@/lib/cn';

import { Button } from '../ui/button';
import { DropdownMenu } from '../ui/dropdown-menu';
import { Tooltip } from '../ui/tooltip';
import { findTrail, isGroup, navigation } from './nav-config';

/** Horizontal layout at ≥lg: groups become menus; below lg the shell uses the same drawer as vertical (plan D24). */
export function TopNav() {
  const t = useTranslations('shell');
  const pathname = usePathname();
  const router = useRouter();
  const trail = findTrail(pathname);
  return (
    <nav aria-label={t('nav.label')} data-top-nav className="flex flex-wrap items-center gap-1 px-4 py-1">
      {navigation.map((entry) => {
        const label = t(`nav.${entry.labelKey}`);
        if (isGroup(entry)) {
          const active = trail?.group?.id === entry.id;
          return (
            <DropdownMenu
              key={entry.id}
              align="start"
              trigger={
                <Button variant="ghost" size="sm" iconEnd={ChevronDown} className={cn(active && 'bg-primary-subtle')}>
                  {label}
                </Button>
              }
              items={entry.children.map((c) => ({
                type: 'item' as const,
                label: t(`nav.${c.labelKey}`),
                icon: c.icon,
                disabled: c.disabled,
                description: c.disabled ? t('notYetAvailable') : undefined,
                onSelect: () => router.push(c.href),
              }))}
            />
          );
        }
        if (entry.disabled) {
          return (
            <Tooltip key={entry.id} content={t('notYetAvailable')}>
              <span role="link" aria-disabled="true" tabIndex={0} className="inline-flex h-8 cursor-not-allowed items-center rounded-md px-3 text-foreground-subtle type-small font-semibold pointer-coarse:min-h-11">
                {label}
              </span>
            </Tooltip>
          );
        }
        const active = trail?.leaf.id === entry.id;
        return (
          <Button key={entry.id} asChild variant="ghost" size="sm" className={cn(active && 'bg-primary-subtle')}>
            <Link href={entry.href} aria-current={active ? 'page' : undefined}>
              {label}
            </Link>
          </Button>
        );
      })}
    </nav>
  );
}
