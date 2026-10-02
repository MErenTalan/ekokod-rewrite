'use client';

import { useLocale, useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import { Dialog } from '@/components/ui/dialog';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Alarm, AlarmEvent } from '@/lib/api/types';
import { formatDateTime, formatNumber } from '@/lib/format';
import type { Locale } from '@/i18n/locale';

import { typeLabelKey } from './alarm-labels';

export type EventsDialogViewProps = {
  open: boolean;
  alarm: Alarm | null;
  events: AlarmEvent[];
  onClose: () => void;
  loading?: boolean;
};

type Delivery = { tone: 'success' | 'danger' | 'neutral'; key: 'delivered' | 'failed' | 'pending' | 'noChannel' };

/** Delivery state of one event: delivered, failed, still queued, or nobody to tell. */
function deliveryTone(event: AlarmEvent, hasChannel: boolean): Delivery {
  if (event.notified_at) return { tone: 'success', key: 'delivered' };
  // A rule with no recipient fired as designed: nothing was meant to be sent.
  if (!hasChannel) return { tone: 'neutral', key: 'noChannel' };
  // A null notified_at WITH an error is "fired and nobody was told" — a
  // different state from one still waiting for the notify task.
  if (event.notification_error) return { tone: 'danger', key: 'failed' };
  return { tone: 'neutral', key: 'pending' };
}

/** Each limit a rule can hold, with the period fields of the reactive ones. */
const LIMITS = [
  { threshold: 'inductive_ratio_threshold', label: 'fields.inductiveRatio', value: 'inductive_period_value', unit: 'inductive_period_unit' },
  { threshold: 'capacitive_ratio_threshold', label: 'fields.capacitiveRatio', value: 'capacitive_period_value', unit: 'capacitive_period_unit' },
  { threshold: 'active_consumption_max', label: 'fields.activeConsumptionMax', value: 'active_consumption_max_period_value', unit: 'active_consumption_max_period_unit' },
  { threshold: 'active_consumption_min', label: 'fields.activeConsumptionMin', value: 'active_consumption_min_period_value', unit: 'active_consumption_min_period_unit' },
  { threshold: 'communication_threshold_hours', label: 'fields.communicationThresholdHours' },
  { threshold: 'power_max', label: 'fields.powerMax' },
  { threshold: 'power_min', label: 'fields.powerMin' },
  { threshold: 'invoice_threshold_pct', label: 'fields.invoiceThresholdPct' },
] as const;

/** The breach sentences the evaluator stored with the event. */
function reasons(event: AlarmEvent): string[] {
  const lines = (event.detail as { lines?: unknown } | null | undefined)?.lines;
  return Array.isArray(lines) ? lines.filter((line): line is string => typeof line === 'string') : [];
}

/**
 * §7.12's "view details" and "view logs" in one dialog, as legacy had them:
 * the rule's own settings summary above its firing history.
 */
export function EventsDialogView({ open, alarm, events, onClose, loading = false }: EventsDialogViewProps) {
  const t = useTranslations('alarms');
  const locale = useLocale() as Locale;
  const settings = (alarm?.settings ?? {}) as Record<string, string | number | null | undefined>;
  const hasChannel = (alarm?.channels ?? []).length > 0;
  const limits = LIMITS.flatMap((limit) => {
    const threshold = settings[limit.threshold];
    if (threshold === undefined || threshold === null || threshold === '') return [];
    const period = 'value' in limit && settings[limit.value] != null
      ? ` — ${t('fields.inLast')} ${settings[limit.value]} ${t(settings[limit.unit] === 'days' ? 'fields.days' : 'fields.hours')}`
      : '';
    return [`${t(limit.label)}: ${formatNumber(String(threshold))}${period}`];
  });

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
            {limits.length > 0 ? (
              <ul className="text-foreground type-body">
                {limits.map((line) => <li key={line}>{line}</li>)}
              </ul>
            ) : null}
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
                  const delivery = deliveryTone(event, hasChannel);
                  return (
                    <TableRow key={event.id}>
                      <TableCell>{formatDateTime(event.triggered_at, locale)}</TableCell>
                      <TableCell>
                        <p className="type-body">{event.message}</p>
                        {reasons(event).map((line) => (
                          <p key={line} className="text-foreground type-caption">{line}</p>
                        ))}
                        {event.notification_error && hasChannel ? (
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
