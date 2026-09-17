'use client';

import { CalendarRange, ChevronLeft, ChevronRight, Plus } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';
import { useState } from 'react';

import { PageHeader } from '@/components/shell/page-header';
import { Button } from '@/components/ui/button';
import { RadioGroup } from '@/components/ui/radio-group';
import { Skeleton } from '@/components/ui/skeleton';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import type { CalendarEvent } from '@/lib/api/types';
import { istanbulToday } from '@/lib/dates';
import { formatDate, formatMonth } from '@/lib/format';
import type { Locale } from '@/i18n/locale';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';
import { DEFAULT_EVENT_COLOUR } from '@/styles/event-palette';

import { AgendaView } from './agenda-view';
import { EventDialogView } from './event-dialog';
import { emptyEvent, eventDraft, toEventRequest, type EventDraft } from './event-draft';
import { MonthView } from './month-view';
import { shiftAnchor, visibleRange, type CalendarView } from './range';
import { TimeGridView } from './time-grid-view';
import { VacationDialogView } from './vacation-dialog';

const VIEWS: CalendarView[] = ['month', 'week', 'day', 'agenda'];

/** The company calendar of 01 §7.18. */
export function CalendarPage() {
  const t = useTranslations('calendar');
  const locale = useLocale() as Locale;
  const { can } = useSession();
  const scope = useScopeParams();
  const [view, setView] = useState<CalendarView>('month');
  const [anchor, setAnchor] = useState(istanbulToday());
  const [draft, setDraft] = useState<EventDraft | null>(null);
  const [vacationsOpen, setVacationsOpen] = useState(false);
  const canEdit = can('calendar.edit');

  const range = visibleRange(view, anchor);
  const events = $api.useQuery('get', '/api/v1/calendar/events', {
    params: { query: { ...scope, from: range.from, to: range.to } },
  });
  const vacations = $api.useQuery('get', '/api/v1/calendar/vacations', { params: { query: scope } });

  const invalidate = ['/api/v1/calendar'];
  const create = useApiMutation('post', '/api/v1/calendar/events', { success: t('saved'), invalidate });
  const update = useApiMutation('patch', '/api/v1/calendar/events/{id}', { success: t('saved'), invalidate });
  const remove = useApiMutation('delete', '/api/v1/calendar/events/{id}', { success: t('deleted'), invalidate });
  const saveVacations = useApiMutation('put', '/api/v1/calendar/vacations', { success: t('vacationsSaved'), invalidate });

  const title =
    view === 'month'
      ? formatMonth(anchor.slice(0, 7), locale)
      : view === 'day'
        ? formatDate(anchor, locale)
        : `${formatDate(range.from, locale)} – ${formatDate(range.to, locale)}`;

  const viewProps = {
    anchor,
    events: events.data?.items ?? [],
    weekendDays: vacations.data?.weekend_days ?? [],
    periods: vacations.data?.periods ?? [],
    canEdit,
    onSelectDate: (date: string) => setDraft(emptyEvent(date, DEFAULT_EVENT_COLOUR)),
    onSelectEvent: (event: CalendarEvent) => setDraft(eventDraft(event, DEFAULT_EVENT_COLOUR)),
  };

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={t('title')}
        description={t('subtitle')}
        actions={
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" iconStart={CalendarRange} onClick={() => setVacationsOpen(true)}>
              {t('vacations')}
            </Button>
            {canEdit ? (
              <Button iconStart={Plus} onClick={() => setDraft(emptyEvent(anchor, DEFAULT_EVENT_COLOUR))}>
                {t('addEvent')}
              </Button>
            ) : null}
          </div>
        }
      />

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Button variant="ghost" iconStart={ChevronLeft} onClick={() => setAnchor(shiftAnchor(view, anchor, -1))}>
            {t('previous')}
          </Button>
          <Button variant="secondary" onClick={() => setAnchor(istanbulToday())}>
            {t('today')}
          </Button>
          <Button variant="ghost" iconEnd={ChevronRight} onClick={() => setAnchor(shiftAnchor(view, anchor, 1))}>
            {t('next')}
          </Button>
          <p className="text-foreground type-h3">{title}</p>
        </div>
        <RadioGroup
          label={t('view')}
          orientation="horizontal"
          options={VIEWS.map((value) => ({ value, label: t(`views.${value}`) }))}
          value={view}
          onValueChange={(next) => setView(next as CalendarView)}
        />
      </div>

      {events.isLoading || vacations.isLoading ? (
        <Skeleton className="h-96 w-full" />
      ) : view === 'month' ? (
        <MonthView {...viewProps} />
      ) : view === 'agenda' ? (
        <AgendaView {...viewProps} />
      ) : (
        <TimeGridView {...viewProps} view={view} />
      )}

      <EventDialogView
        open={draft !== null}
        onOpenChange={(open) => !open && setDraft(null)}
        value={draft}
        onChange={setDraft}
        readOnly={!canEdit}
        saving={create.isPending || update.isPending}
        fieldErrors={{ ...create.fieldErrors, ...update.fieldErrors }}
        onDelete={() => {
          if (draft?.id) remove.mutate({ params: { path: { id: draft.id }, query: scope } });
          setDraft(null);
        }}
        onSubmit={() => {
          if (!draft) return;
          const body = toEventRequest(draft);
          if (draft.id) update.mutate({ params: { path: { id: draft.id }, query: scope }, body });
          else create.mutate({ params: { query: scope }, body });
          setDraft(null);
        }}
      />

      <VacationDialogView
        // The dialog seeds its state from the configuration, so it remounts with it.
        key={`${vacations.data?.weekend_source ?? 'loading'}-${vacations.data?.periods.length ?? 0}`}
        open={vacationsOpen}
        onOpenChange={setVacationsOpen}
        weekendDays={vacations.data?.weekend_days ?? []}
        periods={vacations.data?.periods ?? []}
        readOnly={!canEdit}
        saving={saveVacations.isPending}
        onSave={(values) => {
          saveVacations.mutate({ params: { query: scope }, body: values });
          setVacationsOpen(false);
        }}
      />
    </div>
  );
}
