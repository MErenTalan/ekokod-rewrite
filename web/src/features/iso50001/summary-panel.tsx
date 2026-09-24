'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Skeleton } from '@/components/ui/skeleton';
import { useToast } from '@/components/ui/toast';
import { downloadFile } from '@/lib/api/download';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import { istanbulToday } from '@/lib/dates';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { CalendarDialog } from './calendar-dialog';
import { SummaryView } from './summary-view';

/** R343/R344's data: the project, the clause titles, the dates and the export. */
export function SummaryPanel({ buildingId }: { buildingId: string }) {
  const t = useTranslations('iso');
  const { can } = useSession();
  const scope = useScopeParams();
  const toast = useToast();
  const [calendar, setCalendar] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const path = { params: { path: { building_id: buildingId }, query: scope } };
  const project = $api.useQuery('get', '/api/v1/iso50001/{building_id}', path);
  const clauses = $api.useQuery('get', '/api/v1/iso50001/clauses', { params: { query: scope } });
  const dates = useApiMutation('put', '/api/v1/iso50001/{building_id}/dates', { success: t('calendar.saved'), invalidate: ['/api/v1/iso50001/{building_id}'] });

  const download = async () => {
    setDownloading(true);
    try {
      await downloadFile(`/api/v1/iso50001/${buildingId}/export`, scope, 'ISO-50001.zip');
    } catch {
      toast.toast({ tone: 'danger', title: t('summary.downloadError') });
    } finally {
      setDownloading(false);
    }
  };

  if (!project.data || !clauses.data) return <Skeleton className="h-64 w-full" />;
  const serverError = Object.values(dates.fieldErrors)[0];
  return (
    <>
      <SummaryView
        project={project.data}
        clauses={clauses.data}
        today={istanbulToday()}
        canEdit={can('iso50001.edit')}
        downloading={downloading}
        onDownload={() => void download()}
        onCalendar={() => setCalendar(true)}
      />
      <CalendarDialog
        open={calendar}
        project={project.data}
        clauses={clauses.data}
        saving={dates.isPending}
        error={serverError}
        onClose={() => setCalendar(false)}
        onSave={(rows) => dates.mutate({ ...path, body: { clauses: rows } }, { onSuccess: () => setCalendar(false) })}
      />
    </>
  );
}
