'use client';

import { AlertTriangle, Clock, Server } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { OperationalMessage } from '@/lib/api/types';
import { formatDateTime } from '@/lib/format';
import type { Locale } from '@/i18n/locale';

export type MessageTableViewProps = {
  messages: OperationalMessage[];
  /** True when any filter narrows the list, so the empty state can say why. */
  filtered: boolean;
  loading?: boolean;
};

const KIND_ICON = { alarm: AlertTriangle, job: Clock, system: Server } as const;
const KIND_LABEL = { alarm: 'types.alarm', job: 'types.job', system: 'types.system' } as const;
const STATUS_TONE = {
  success: 'success', error: 'danger', warning: 'warning', info: 'neutral',
} as const;
const STATUS_LABEL = {
  success: 'statuses.success', error: 'statuses.error', warning: 'statuses.warning', info: 'statuses.info',
} as const;

/** The §7.13 table: type badge, status badge, message, detail, related entity
 *  and timestamp. */
export function MessageTableView({ messages, filtered, loading = false }: MessageTableViewProps) {
  const t = useTranslations('messages');
  const locale = useLocale() as Locale;

  if (loading) return <Skeleton className="h-64 w-full" />;
  if (messages.length === 0) {
    return (
      <EmptyState
        title={filtered ? t('emptyFiltered') : t('empty')}
        description={filtered ? t('emptyFilteredDescription') : t('emptyDescription')}
      />
    );
  }

  return (
    <TableContainer label={t('title')}>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('table.type')}</TableHead>
            <TableHead>{t('table.message')}</TableHead>
            <TableHead>{t('table.details')}</TableHead>
            <TableHead>{t('table.related')}</TableHead>
            <TableHead>{t('table.status')}</TableHead>
            <TableHead>{t('table.date')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {messages.map((message) => {
            const Icon = KIND_ICON[message.kind as keyof typeof KIND_ICON] ?? Server;
            return (
              <TableRow key={message.id}>
                <TableCell>
                  <span className="flex items-center gap-2">
                    <Icon aria-hidden className="size-4 text-foreground-muted" />
                    {t(KIND_LABEL[message.kind as keyof typeof KIND_LABEL] ?? 'types.system')}
                  </span>
                </TableCell>
                <TableCell>
                  <p className="type-body">{message.message}</p>
                  <p className="text-foreground-muted type-caption">
                    {t('table.category')}: {message.category}
                  </p>
                </TableCell>
                <TableCell className="max-w-sm break-words">
                  {message.detail ? <span className="type-body">{message.detail}</span> : '—'}
                </TableCell>
                <TableCell>
                  {/* Both null is an ordinary state: a message about nothing in
                      particular, such as a run summary. */}
                  {message.related_type ? (
                    <span className="type-body">{message.related_type}</span>
                  ) : (
                    '—'
                  )}
                </TableCell>
                <TableCell>
                  <Badge tone={STATUS_TONE[message.status as keyof typeof STATUS_TONE] ?? 'neutral'}>
                    {t(STATUS_LABEL[message.status as keyof typeof STATUS_LABEL] ?? 'statuses.info')}
                  </Badge>
                </TableCell>
                <TableCell>{formatDateTime(message.created_at, locale)}</TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </TableContainer>
  );
}
