'use client';

import { useTranslations } from 'next-intl';
import { useMemo } from 'react';

import type { ValueLabels } from './format';

/** The `reports.value` words as the pure formatters take them. */
export function useValueLabels(subject: 'buildings' | 'plants' = 'buildings'): ValueLabels {
  const t = useTranslations('reports.value');
  return useMemo(
    () => ({
      noData: t('noData'),
      notSelected: t('notSelected'),
      perKwh: t('perKwh'),
      coverage: (withData: number, of: number) => t('coverage', { withData, of, subject: t(subject) }),
    }),
    [t, subject],
  );
}
