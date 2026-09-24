'use client';

import { Monitor, Moon, Sun } from 'lucide-react';
import { useTranslations } from 'next-intl';

import type { UiPreferences } from '@/lib/ui-preferences';
import { useUiPreferences } from '@/lib/ui-preferences-provider';

import { IconButton } from '../ui/icon-button';

const next: Record<UiPreferences['theme'], UiPreferences['theme']> = { light: 'dark', dark: 'system', system: 'light' };
const icons = { light: Sun, dark: Moon, system: Monitor } as const;

export function ThemeToggle({ className }: { className?: string }) {
  const t = useTranslations('shell');
  const { prefs, update } = useUiPreferences();
  return (
    <IconButton
      className={className}
      label={t('theme.toggle', { mode: t(`theme.${prefs.theme}`) })}
      icon={icons[prefs.theme]}
      onClick={() => update({ theme: next[prefs.theme] })}
    />
  );
}
