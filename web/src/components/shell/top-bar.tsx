'use client';

import { Bell, Leaf, PanelLeft, PanelLeftClose, Search, Settings2 } from 'lucide-react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { useUiPreferences } from '@/lib/ui-preferences-provider';

import { IconButton } from '../ui/icon-button';
import { Popover } from '../ui/popover';
import { SearchInput } from '../ui/search-input';
import { LanguageSwitcher } from './language-switcher';
import { ThemeToggle } from './theme-toggle';
import { UserMenu, type ShellUser } from './user-menu';

export type TopBarProps = {
  user: ShellUser;
  notificationCount?: number;
  sidebarOpen: boolean;
  onToggleSidebar: () => void;
  onOpenCustomizer: () => void;
};

export function TopBar({ user, notificationCount, sidebarOpen, onToggleSidebar, onOpenCustomizer }: TopBarProps) {
  const t = useTranslations('shell');
  const common = useTranslations('common');
  const app = useTranslations('app');
  const { prefs, update } = useUiPreferences();
  const [query, setQuery] = useState('');
  const collapsed = prefs.sidebar === 'collapsed';
  return (
    <header className="sticky top-0 z-20 flex h-14 items-center gap-1 border-b border-border bg-surface px-2 sm:gap-2 sm:px-3">
      <IconButton className="lg:hidden" label={t('toggleSidebar')} icon={PanelLeft} aria-expanded={sidebarOpen} onClick={onToggleSidebar} />
      {prefs.layout === 'vertical' ? (
        <IconButton
          className="hidden lg:inline-flex"
          label={t('collapseSidebar')}
          icon={collapsed ? PanelLeft : PanelLeftClose}
          aria-expanded={!collapsed}
          onClick={() => update({ sidebar: collapsed ? 'expanded' : 'collapsed' })}
        />
      ) : null}
      <Link href="/dashboard" aria-label={t('home')} className="flex items-center gap-1.5 rounded-md px-1 pointer-coarse:min-h-11 pointer-coarse:min-w-11">
        <Leaf aria-hidden className="size-6 stroke-brand" />
        <span className="hidden font-heading text-lg font-semibold text-foreground sm:inline">{app('name')}</span>
      </Link>
      <div className="ms-auto hidden w-72 md:block">
        <SearchInput label={t('search')} labelVisibility="hidden" placeholder={t('searchPlaceholder')} value={query} onValueChange={setQuery} />
      </div>
      <div className="ms-auto flex items-center gap-0.5 sm:gap-1 md:ms-2">
        <Popover label={t('search')} align="end" trigger={<IconButton className="md:hidden" label={t('search')} icon={Search} />}>
          <div className="w-[min(20rem,calc(100vw-32px))]">
            <SearchInput label={t('search')} placeholder={t('searchPlaceholder')} value={query} onValueChange={setQuery} />
          </div>
        </Popover>
        <span className="relative inline-flex">
          <IconButton label={notificationCount ? t('notificationsCount', { count: notificationCount }) : common('notifications')} icon={Bell} />
          {notificationCount ? (
            <span aria-hidden className="pointer-events-none absolute end-0.5 top-0.5 min-w-4 rounded-full bg-danger px-1 text-center text-on-danger type-caption leading-4">
              {notificationCount > 99 ? '99+' : notificationCount}
            </span>
          ) : null}
        </span>
        <LanguageSwitcher />
        <ThemeToggle className="hidden sm:inline-flex" />
        <IconButton className="hidden sm:inline-flex" label={t('customizer.open')} icon={Settings2} onClick={onOpenCustomizer} />
        <UserMenu user={user} onOpenCustomizer={onOpenCustomizer} />
      </div>
    </header>
  );
}
