'use client';

import { useLocale, useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import { Dialog } from '@/components/ui/dialog';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Alarm, AlarmEvent } from '@/lib/api/types';
import { formatDateTime } from '@/lib/format';
import type { Locale } from '@/i18n/locale';

import { typeLabelKey } from './alarm-labels';

export type EventsDialogViewProps = {
  open: boolean;
  alarm: Alarm | null;
  events: AlarmEvent[];
  onClose: () => void;
  loading?: boolean;
};

/** Delivery state of one event: delivered, failed, or still queued. */
function deliveryTone(event: AlarmEvent): { tone: 'success' | 'danger' | 'neutral'; key: 'delivered' | 'failed' | 'pending' } {
  if (event.notified_at) return { tone: 'success', key: 'delivered' };
  // A null notified_at WITH an error is "fired and nobody was told" — a
  // different state from one still waiting for the notify task.
  if (event.notification_error) return { tone: 'danger', key: 'failed' };
  return { tone: 'neutral', key: 'pending' };
}

/**
 * §7.12's "view details" and "view logs" in one dialog, as legacy had them:
 * the rule's own settings summary above its firing history.
 */
export function EventsDialogView({ open, alarm, events, onClose, loading = false }: EventsDialogViewProps) {
  const t = useTranslations('alarms');
  const locale = useLocale() as Locale;

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => !next && onClose()}
      title={alarm ? `${t('events.title')} — ${alarm.name}` : t('events.title')}
      size="lg"
    >
      <div className="flex flex-col gap-4">
        {alarm ? (
          <section className="flex flex-col gap-1">
            <h3 className="text-foreground type-h3">{t('events.settingsTitle')}</h3>
            <p className="text-foreground-muted type-body">{t(typeLabelKey(alarm.type))}</p>
            <div className="flex flex-wrap gap-1">
              {(alarm.analyzers ?? []).map((a) => (
                <Badge key={a.id} tone="neutral">{a.installation_number}</Badge>
              ))}
            </div>
          </section>
        ) : null}

        {loading ? (
          <Skeleton className="h-48 w-full" />
        ) : events.length === 0 ? (
          <EmptyState title={t('events.empty')} description={t('events.emptyDescription')} />
        ) : (
          <TableContainer label={t('events.title')}>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('events.triggeredAt')}</TableHead>
                  <TableHead>{t('events.message')}</TableHead>
                  <TableHead>{t('events.delivery')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {events.map((event) => {
                  const delivery = deliveryTone(event);
                  return (
                    <TableRow key={event.id}>
                      <TableCell>{formatDateTime(event.triggered_at, locale)}</TableCell>
                      <TableCell>
                        <p className="type-body">{event.message}</p>
                        {event.notification_error ? (
                          <p className="text-foreground-muted type-caption">{event.notification_error}</p>
                        ) : null}
                      </TableCell>
                      <TableCell>
                        <Badge tone={delivery.tone}>{t(`events.${delivery.key}`)}</Badge>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </div>
    </Dialog>
  );
}
