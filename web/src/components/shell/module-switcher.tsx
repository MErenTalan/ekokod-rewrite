'use client';

import { LayoutGrid } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { CHOOSER_PATH } from '@/lib/api/errors';
import { MODULES, type ModuleId } from '@/modules/registry';

import { DropdownMenu } from '../ui/dropdown-menu';
import { IconButton } from '../ui/icon-button';

/** R496: jump between modules on the same session (no second sign-in); the current one is shown, not offered. */
export function ModuleSwitcher({ current, modules }: { current: ModuleId; modules: ModuleId[] }) {
  const t = useTranslations('shell.modules');
  const router = useRouter();
  return (
    <DropdownMenu
      align="start"
      trigger={<IconButton label={t('switcher')} icon={LayoutGrid} />}
      items={[
        { type: 'label', label: t('label') },
        ...modules.map((id) => ({
          type: 'item' as const,
          label: `${t(`${id}.name`)} · ${t(`${id}.title`)}`,
          icon: MODULES[id].icon,
          disabled: id === current,
          description: id === current ? t('current') : undefined,
          onSelect: () => router.push(MODULES[id].home),
        })),
        { type: 'separator' },
        { type: 'item', label: t('all'), icon: LayoutGrid, onSelect: () => router.push(CHOOSER_PATH) },
      ]}
    />
  );
}
