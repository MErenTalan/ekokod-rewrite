'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { PageHeader } from '@/components/shell/page-header';
import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import type { Alarm } from '@/lib/api/types';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { AlarmDialogView } from './alarm-dialog';
import { draftFrom, emptyDraft, toAlarmRequest, type AlarmDraft } from './alarm-draft';
import { AlarmTableView, type AlarmStateFilter } from './alarm-table';
import { EvaluateDialogView } from './evaluate-dialog';
import { EventsDialogView } from './events-dialog';

const LIST_LIMIT = 200;

/** The alarm rules of 01 §7.12. */
export function AlarmsPage() {
  const t = useTranslations('alarms');
  const common = useTranslations('common');
  const { can } = useSession();
  const scope = useScopeParams();

  const [state, setState] = useState<AlarmStateFilter>('all');
  const [draft, setDraft] = useState<AlarmDraft | null>(null);
  const [deleting, setDeleting] = useState<Alarm | null>(null);
  const [details, setDetails] = useState<Alarm | null>(null);
  const [evaluating, setEvaluating] = useState<Alarm | null>(null);

  const canEdit = can('alarms.edit');
  const canEvaluate = can('alarms.evaluate');

  const query = {
    ...scope,
    limit: LIST_LIMIT,
    ...(state === 'all' ? {} : { is_enabled: state === 'active' }),
  };
  const alarms = $api.useQuery('get', '/api/v1/alarms', { params: { query } });

  // The dialog's analyzer picker only needs what the caller may attach, which
  // is exactly what the scoped list returns.
  const analyzers = $api.useQuery(
    'get',
    '/api/v1/analyzers',
    { params: { query: { ...scope, limit: 500 } } },
    { enabled: draft !== null },
  );

  const events = $api.useQuery(
    'get',
    '/api/v1/alarms/{id}/events',
    { params: { path: { id: details?.id ?? '' }, query: { ...scope, limit: 100 } } },
    { enabled: details !== null },
  );

  const invalidate = ['/api/v1/alarms'];
  const create = useApiMutation('post', '/api/v1/alarms', { success: t('saved'), invalidate });
  const update = useApiMutation('patch', '/api/v1/alarms/{id}', { success: t('saved'), invalidate });
  const remove = useApiMutation('delete', '/api/v1/alarms/{id}', { success: t('deleted'), invalidate });
  const evaluate = useApiMutation('post', '/api/v1/alarms/{id}/evaluate', { invalidate: [] });

  const submit = () => {
    if (!draft) return;
    const body = toAlarmRequest(draft);
    const done = () => setDraft(null);
    if (draft.id) {
      update.mutate({ params: { path: { id: draft.id }, query: scope }, body }, { onSuccess: done });
    } else {
      create.mutate({ params: { query: scope }, body }, { onSuccess: done });
    }
  };

  const toggle = (alarm: Alarm, enabled: boolean) => {
    // The toggle is a full replace like any other edit, so the rule's own
    // settings travel with it rather than being cleared by an empty body.
    const next = { ...draftFrom(alarm), isEnabled: enabled };
    update.mutate({ params: { path: { id: alarm.id }, query: scope }, body: toAlarmRequest(next) });
  };

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} />

      <AlarmTableView
        alarms={alarms.data?.items ?? []}
        state={state}
        onStateChange={setState}
        canEdit={canEdit}
        canEvaluate={canEvaluate}
        loading={alarms.isPending}
        onCreate={() => setDraft(emptyDraft())}
        onEdit={(alarm) => setDraft(draftFrom(alarm))}
        onDelete={setDeleting}
        onToggle={toggle}
        onDetails={setDetails}
        onEvaluate={(alarm) => {
          setEvaluating(alarm);
          evaluate.mutate({ params: { path: { id: alarm.id }, query: scope } });
        }}
      />

      {draft ? (
        <AlarmDialogView
          open
          draft={draft}
          analyzers={analyzers.data?.items ?? []}
          onDraftChange={setDraft}
          onSubmit={submit}
          onClose={() => setDraft(null)}
          saving={create.isPending || update.isPending}
        />
      ) : null}

      <EventsDialogView
        open={details !== null}
        alarm={details}
        events={events.data?.items ?? []}
        loading={events.isPending}
        onClose={() => setDetails(null)}
      />

      <EvaluateDialogView
        open={evaluating !== null}
        alarmName={evaluating?.name ?? ''}
        evaluation={evaluate.data ?? null}
        loading={evaluate.isPending}
        onClose={() => setEvaluating(null)}
      />

      <Dialog
        open={deleting !== null}
        onOpenChange={(next) => !next && setDeleting(null)}
        title={t('deleteTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeleting(null)}>{common('cancel')}</Button>
            <Button
              variant="danger"
              loading={remove.isPending}
              onClick={() => {
                if (!deleting) return;
                remove.mutate(
                  { params: { path: { id: deleting.id }, query: scope } },
                  { onSuccess: () => setDeleting(null) },
                );
              }}
            >
              {common('delete')}
            </Button>
          </>
        }
      >
        <p className="type-body">{t('deleteConfirm', { name: deleting?.name ?? '' })}</p>
      </Dialog>
    </div>
  );
}
