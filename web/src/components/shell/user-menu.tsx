'use client';

import { ChevronDown, LogOut, SlidersHorizontal, UserRound } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { useLogout } from '@/lib/session/use-logout';

import { DropdownMenu } from '../ui/dropdown-menu';

export type ShellUser = { name: string; email: string; roleLabel: string };

/** Identity, profile, display settings and logout (R168: logout ends the server session). */
export function UserMenu({ user, onOpenCustomizer }: { user: ShellUser; onOpenCustomizer?: () => void }) {
  const t = useTranslations('shell');
  const router = useRouter();
  const logout = useLogout();
  const initials = user.name
    .split(/\s+/)
    .map((part) => part[0])
    .slice(0, 2)
    .join('')
    .toLocaleUpperCase('tr');
  return (
    <DropdownMenu
      trigger={
        <button
          type="button"
          aria-label={t('userMenu.label', { name: user.name })}
          className="inline-flex h-9 items-center gap-2 rounded-md px-1.5 text-foreground hover:bg-surface-sunken pointer-coarse:min-h-11 pointer-coarse:min-w-11"
        >
          <span aria-hidden className="flex size-7 items-center justify-center rounded-full bg-primary-subtle text-primary type-caption">
            {initials}
          </span>
          <span className="hidden max-w-32 truncate type-small md:inline">{user.name}</span>
          <ChevronDown aria-hidden className="hidden size-4 text-foreground-muted md:block" />
        </button>
      }
      items={[
        { type: 'label', label: user.name },
        { type: 'label', label: `${user.roleLabel} · ${user.email}` },
        { type: 'separator' },
        { type: 'item', label: t('userMenu.profile'), icon: UserRound, onSelect: () => router.push('/ekorm/settings') },
        { type: 'item', label: t('userMenu.displaySettings'), icon: SlidersHorizontal, onSelect: () => onOpenCustomizer?.() },
        { type: 'separator' },
        { type: 'item', label: t('userMenu.logout'), icon: LogOut, onSelect: () => void logout() },
      ]}
    />
  );
}
