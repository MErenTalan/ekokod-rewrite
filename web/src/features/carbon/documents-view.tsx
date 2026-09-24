'use client';

import { useTranslations } from 'next-intl';

import { EmptyState } from '@/components/ui/empty-state';

/** Q-F9: 05 §12 has no document routes yet; the tab says so rather than offering an upload that goes nowhere. */
export function DocumentsView() {
  const t = useTranslations('carbon');
  return <EmptyState title={t('documents.title')} description={t('documents.hint')} />;
}
