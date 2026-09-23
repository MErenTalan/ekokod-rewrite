'use client';

import { useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import { EmptyState } from '@/components/ui/empty-state';
import { Button } from '@/components/ui/button';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tooltip } from '@/components/ui/tooltip';
import type { PlantFault } from '@/lib/api/types';

import { shortDateTime } from './format';

export const ALARM_PAGE = 50;
const LEVEL_TONE = { 1: 'danger', 2: 'warning', 3: 'info', 4: 'neutral' } as const;
const LEVELS = { 1: 'critical', 2: 'major', 3: 'minor', 4: 'warning' } as const;
const TYPES = { 1: 'fault', 2: 'alarm', 3: 'notice', 4: 'suggestion' } as const;
const known = (v?: number | null): v is 1 | 2 | 3 | 4 => v === 1 || v === 2 || v === 3 || v === 4;

export type AlarmsTabViewProps = { items: PlantFault[]; total: number; onLoadMore?: () => void; loadingMore?: boolean };

/** §7.7's alarm list (R286): translated text, level, type, open/closed, and an untranslated marker. */
export function AlarmsTabView({ items, total, onLoadMore, loadingMore = false }: AlarmsTabViewProps) {
  const t = useTranslations('solarPlants');
  if (items.length === 0) return <EmptyState title={t('alarms.empty')} description={t('alarms.emptyDescription')} />;
  return (
    <div className="flex flex-col gap-4">
      <TableContainer label={t('alarms.title')}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('alarms.when')}</TableHead>
              <TableHead>{t('alarms.device')}</TableHead>
              <TableHead>{t('alarms.message')}</TableHead>
              <TableHead>{t('alarms.level')}</TableHead>
              <TableHead>{t('alarms.type')}</TableHead>
              <TableHead>{t('alarms.closed')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((a) => (
              <TableRow key={a.ref}>
                <TableCell>{shortDateTime(a.occurred_at)}</TableCell>
                <TableCell>{a.device_name ?? '—'}</TableCell>
                <TableCell>
                  <span className="flex flex-wrap items-center gap-2">
                    {a.message_tr}
                    {a.translated ? null : (
                      <span data-touch-target className="inline-flex items-center pointer-coarse:min-h-11">
                        <Tooltip content={t('alarms.untranslatedHint')}>
                          <Badge tone="neutral" tabIndex={0}>{t('alarms.untranslated')}</Badge>
                        </Tooltip>
                      </span>
                    )}
                  </span>
                </TableCell>
                <TableCell>
                  <Badge tone={known(a.level) ? LEVEL_TONE[a.level] : 'neutral'}>
                    {t(`alarms.levels.${known(a.level) ? LEVELS[a.level] : 'other'}`)}
                  </Badge>
                </TableCell>
                <TableCell>{t(`alarms.types.${known(a.type) ? TYPES[a.type] : 'other'}`)}</TableCell>
                <TableCell>{a.closed_at ? shortDateTime(a.closed_at) : t('alarms.open')}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-foreground-muted type-small">{t('alarms.shown', { shown: items.length, total })}</p>
        {onLoadMore ? <Button variant="secondary" onClick={onLoadMore} disabled={loadingMore}>{t('alarms.more')}</Button> : null}
      </div>
    </div>
  );
}
