'use client';

import { useTranslations } from 'next-intl';
import { useEffect, useState } from 'react';

import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { DateRangePicker, type DateRange } from '@/components/ui/date-range-picker';
import { Dialog } from '@/components/ui/dialog';
import type { ISOClauses, ISOProject } from '@/lib/api/types';

export type ClauseDatesBody = { clause_id: string; start?: string; end?: string };

export type CalendarDialogProps = {
  open: boolean;
  project: ISOProject;
  clauses: ISOClauses;
  saving: boolean;
  /** A server refusal (R332), already translated. */
  error?: string;
  onSave: (rows: ClauseDatesBody[]) => void;
  onClose: () => void;
};

/** R344: each main clause's start and end; saving needs all five. */
export function CalendarDialog({ open, project, clauses, saving, error, onSave, onClose }: CalendarDialogProps) {
  const t = useTranslations('iso');
  const [ranges, setRanges] = useState<Record<string, DateRange | null>>({});
  useEffect(() => {
    if (!open) return;
    setRanges(Object.fromEntries(project.clauses.map((c) => [c.clause_id, c.start && c.end ? { from: c.start, to: c.end } : null])));
  }, [open, project]);
  const complete = project.clauses.every((c) => ranges[c.clause_id]);
  const title = (id: string) => clauses.items.find((c) => c.id === id)?.title ?? id;
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('calendar.title')}
      description={t('calendar.subtitle')}
      size="lg"
      footer={
        <Button
          loading={saving}
          disabled={!complete}
          onClick={() => onSave(project.clauses.map((c) => ({ clause_id: c.clause_id, start: ranges[c.clause_id]?.from, end: ranges[c.clause_id]?.to })))}
        >
          {t('calendar.save')}
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        {error ? <Alert tone="danger" title={error} /> : null}
        {!complete ? <p className="text-foreground-muted type-small">{t('calendar.errorAll')}</p> : null}
        {project.clauses.map((c) => (
          <DateRangePicker
            key={c.clause_id}
            label={title(c.clause_id)}
            value={ranges[c.clause_id] ?? null}
            onValueChange={(v) => setRanges((prev) => ({ ...prev, [c.clause_id]: v }))}
            required
          />
        ))}
      </div>
    </Dialog>
  );
}
