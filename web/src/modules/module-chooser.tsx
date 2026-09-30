'use client';

import { ArrowRight, Leaf, LogOut } from 'lucide-react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/cn';
import { useLogout } from '@/lib/session/use-logout';

import { MODULES, type ModuleId } from './registry';

/** R495: the post-login landing — one card per module the user may open, one shared session behind all of them. */
export function ModuleChooser({ name, modules }: { name: string; modules: ModuleId[] }) {
  const t = useTranslations('shell');
  const app = useTranslations('app');
  const logout = useLogout();
  return (
    <div className="min-h-dvh bg-background text-foreground">
      <header className="flex h-14 items-center gap-2 border-b border-border bg-surface px-4">
        <Leaf aria-hidden className="size-6 stroke-brand" />
        <span className="font-heading text-lg font-semibold">{app('name')}</span>
        <Button variant="ghost" size="sm" iconStart={LogOut} className="ms-auto" onClick={() => void logout()}>
          {t('userMenu.logout')}
        </Button>
      </header>
      <main id="main-content" className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-10">
        <div className="flex flex-col gap-2">
          <h1 className="type-h1">{t('modules.chooser.title')}</h1>
          <p className="text-foreground-muted type-body">{t('modules.chooser.description', { name })}</p>
        </div>
        <ul className="grid gap-4 sm:grid-cols-2">
          {modules.map((id) => {
            const { home, icon: Icon, iconClass } = MODULES[id];
            const moduleName = t(`modules.${id}.name`);
            return (
              <li key={id}>
                <Link
                  href={home}
                  aria-label={t('modules.chooser.open', { name: moduleName })}
                  className="group flex h-full flex-col gap-3 rounded-lg border border-border bg-surface-raised p-5 transition-colors duration-(--duration-hover) hover:border-border-control"
                >
                  <span className="flex items-center gap-3">
                    <Icon aria-hidden className={cn('size-8', iconClass)} />
                    <span className="flex flex-col">
                      <span className="font-heading text-lg font-semibold">{moduleName}</span>
                      <span className="text-foreground-muted type-small">{t(`modules.${id}.title`)}</span>
                    </span>
                    <ArrowRight aria-hidden className="ms-auto size-5 text-foreground-muted transition-transform duration-(--duration-hover) group-hover:translate-x-0.5" />
                  </span>
                  <span className="text-foreground-muted type-body">{t(`modules.${id}.description`)}</span>
                </Link>
              </li>
            );
          })}
        </ul>
      </main>
    </div>
  );
}
