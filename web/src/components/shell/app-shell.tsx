'use client';

import { useTranslations } from 'next-intl';
import { useEffect, useState, type ReactNode } from 'react';

import { cn } from '@/lib/cn';
import { useUiPreferences } from '@/lib/ui-preferences-provider';

import { Drawer } from '../ui/drawer';
import { Sidebar } from './sidebar';
import { ThemeCustomizer } from './theme-customizer';
import { TopBar } from './top-bar';
import { TopNav } from './top-nav';
import type { ShellUser } from './user-menu';
import { useMediaQuery } from './use-media-query';

export type AppShellProps = { children: ReactNode; user: ShellUser; notificationCount?: number };

/**
 * Dashboard shell (07 §7). Docking is CSS-first so SSR and first paint match at every width (plan I-16);
 * preferences come from the cookie-backed provider, the breadcrumb from PageHeader.
 */
export function AppShell({ children, user, notificationCount }: AppShellProps) {
  const t = useTranslations('shell');
  const { prefs } = useUiPreferences();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [customizerOpen, setCustomizerOpen] = useState(false);
  const desktop = useMediaQuery('(min-width: 1024px)');
  useEffect(() => {
    if (desktop) setDrawerOpen(false);
  }, [desktop]);

  return (
    <div className="min-h-dvh bg-background text-foreground">
      <a
        href="#main-content"
        data-skip-link
        onClick={(e) => {
          e.preventDefault();
          document.getElementById('main-content')?.focus();
        }}
        // Box styles apply only while focused, so the hidden link stays a true 1×1 sr-only element.
        className="sr-only focus:not-sr-only focus:fixed focus:start-2 focus:top-2 focus:z-50 focus:inline-flex focus:min-h-11 focus:items-center focus:rounded-md focus:bg-primary focus:px-3 focus:text-on-primary"
      >
        {t('skipToContent')}
      </a>
      <TopBar
        user={user}
        notificationCount={notificationCount}
        sidebarOpen={drawerOpen}
        onToggleSidebar={() => setDrawerOpen(true)}
        onOpenCustomizer={() => setCustomizerOpen(true)}
      />
      {prefs.layout === 'horizontal' ? (
        <div className="hidden border-b border-border bg-surface lg:block">
          <TopNav />
        </div>
      ) : null}
      <div className="flex">
        {prefs.layout === 'vertical' ? (
          <aside data-docked-sidebar className="sticky top-14 hidden h-[calc(100dvh-3.5rem)] shrink-0 overflow-y-auto border-e border-border bg-surface lg:block">
            <Sidebar collapsed={prefs.sidebar === 'collapsed'} />
          </aside>
        ) : null}
        <main id="main-content" tabIndex={-1} className="min-w-0 flex-1 px-4 py-6 lg:px-6">
          <div className={cn('mx-auto flex w-full flex-col gap-6', prefs.container === 'boxed' ? 'max-w-7xl' : 'max-w-[1600px]')}>{children}</div>
        </main>
      </div>
      {drawerOpen ? (
        <Drawer open side="start" title={t('nav.label')} onOpenChange={setDrawerOpen}>
          <Sidebar collapsed={false} onNavigate={() => setDrawerOpen(false)} />
        </Drawer>
      ) : null}
      <ThemeCustomizer open={customizerOpen} onOpenChange={setCustomizerOpen} />
    </div>
  );
}
