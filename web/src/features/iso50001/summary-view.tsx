'use client';

import { Download } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { ProgressBar } from '@/components/ui/progress-bar';
import type { ISOClauses, ISOProject } from '@/lib/api/types';

import { GanttView } from './gantt-view';

export type SummaryViewProps = {
  project: ISOProject;
  clauses: ISOClauses;
  today: string;
  canEdit: boolean;
  downloading: boolean;
  onDownload: () => void;
  onCalendar: () => void;
};

/** R343: the project home. */
export function SummaryView({ project, clauses, today, canEdit, downloading, onDownload, onCalendar }: SummaryViewProps) {
  const t = useTranslations('iso');
  const counts = new Map(project.counts.map((c) => [c.clause_id, c]));
  const done = project.counts.filter((c) => c.notes > 0 || c.files > 0).length;
  const hasDates = project.clauses.some((c) => c.start);
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-center gap-3">
        <Button variant="secondary" loading={downloading} onClick={onDownload}>
          <Download aria-hidden className="size-4" />
          {downloading ? t('summary.downloading') : t('summary.download')}
        </Button>
        {canEdit ? (
          <Button variant="secondary" onClick={onCalendar}>
            {hasDates ? t('summary.updateCalendar') : t('summary.setDates')}
          </Button>
        ) : null}
      </div>
      <div className="flex flex-col gap-2 rounded-lg border border-border bg-surface-raised p-4">
        <ProgressBar label={t('progress.title')} value={project.progress} valueText={`%${project.progress}`} tone="success" />
        <p className="text-foreground-muted type-small">{t('progress.value', { done, total: project.counts.length })}</p>
      </div>
      <GanttView project={project} clauses={clauses} today={today} />
      <section aria-labelledby="iso-summary" className="flex flex-col gap-4">
        <h2 id="iso-summary" className="type-h3">
          {t('summary.title')}
        </h2>
        <div className="grid gap-4 lg:grid-cols-2">
          {clauses.items.map((main) => (
            <div key={main.id} className="flex flex-col gap-2 rounded-lg border border-border bg-surface-raised p-4">
              <h3 className="type-body font-semibold">{main.title}</h3>
              <ul className="flex flex-col gap-2">
                {main.subs.map((sub) => {
                  const c = counts.get(sub.id) ?? { notes: 0, files: 0 };
                  return (
                    <li key={sub.id} aria-label={sub.title} className="flex flex-col gap-0.5 border-t border-border pt-2">
                      <span className="text-foreground type-small">{sub.title}</span>
                      <span className="text-foreground-muted type-caption">
                        {c.notes > 0 ? t('summary.notes', { count: c.notes }) : t('summary.noNotes')}
                        {' · '}
                        {c.files > 0 ? t('summary.files', { count: c.files }) : t('summary.noFiles')}
                      </span>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
