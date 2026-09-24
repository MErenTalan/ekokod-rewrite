'use client';

import { useLocale, useTranslations } from 'next-intl';

import { StatusBadge } from '@/components/ui/status-badge';
import type { Locale } from '@/i18n/locale';
import type { ISOClauses, ISOProject } from '@/lib/api/types';
import { formatDate } from '@/lib/format';

const DAY = 86_400_000;
const TONE = { not_started: 'neutral', in_progress: 'info', completed: 'success', expired: 'danger' } as const;
const KEY = { not_started: 'notStarted', in_progress: 'inProgress', completed: 'completed', expired: 'expired' } as const;

/** A clause's bar on the project axis, in percent; a one-day clause still shows. */
export function barGeometry(start: string, end: string, projectStart: string, projectEnd: string): { left: number; width: number } {
  const span = (Date.parse(projectEnd) - Date.parse(projectStart)) / DAY + 1;
  const left = ((Date.parse(start) - Date.parse(projectStart)) / DAY / span) * 100;
  const width = (((Date.parse(end) - Date.parse(start)) / DAY + 1) / span) * 100;
  return { left, width: Math.max(width, 0.5) };
}

/** R334's timeline: bars on a shared axis, each row also carrying its dates and status as text (Q-G7). */
export function GanttView({ project, clauses, today }: { project: ISOProject; clauses: ISOClauses; today: string }) {
  const t = useTranslations('iso');
  const locale = useLocale() as Locale;
  const title = (id: string) => clauses.items.find((c) => c.id === id)?.title ?? id;
  const start = project.project_start;
  const end = project.project_end;
  return (
    <section aria-labelledby="iso-gantt" className="flex flex-col gap-3 rounded-lg border border-border bg-surface-raised p-4">
      <h2 id="iso-gantt" className="type-h3">
        {t('gantt.title')}
      </h2>
      {!project.gantt_available || !start || !end ? (
        <p className="text-foreground-muted type-body">{t('gantt.noData')}</p>
      ) : (
        <>
          <p className="flex flex-wrap justify-between gap-2 text-foreground-muted type-small">
            <span>{`${t('gantt.start')}: ${formatDate(start, locale)}`}</span>
            <span>{`${t('gantt.end')}: ${formatDate(end, locale)}`}</span>
          </p>
          <ul aria-label={t('gantt.label')} className="flex flex-col gap-3">
            {project.clauses.map((c) => {
              if (!c.start || !c.end) return null;
              const bar = barGeometry(c.start, c.end, start, end);
              const status = c.status ?? 'not_started';
              return (
                <li key={c.clause_id} className="grid gap-2 sm:grid-cols-[12rem_1fr] sm:items-center">
                  <div className="flex flex-col gap-1">
                    <span className="text-foreground type-small font-semibold">{title(c.clause_id)}</span>
                    <span className="text-foreground-muted type-caption">
                      {t('gantt.range', { start: formatDate(c.start, locale), end: formatDate(c.end, locale) })}
                    </span>
                    <StatusBadge status={TONE[status]} label={t(`gantt.status.${KEY[status]}`)} />
                  </div>
                  <div className="relative h-4 rounded-sm bg-surface-sunken" aria-hidden>
                    <div
                      className={`absolute inset-y-0 rounded-sm ${status === 'completed' ? 'bg-success' : status === 'expired' ? 'bg-danger' : status === 'in_progress' ? 'bg-info' : 'bg-border-strong'}`}
                      style={{ left: `${bar.left}%`, width: `${bar.width}%` }}
                    />
                    {today >= start && today <= end ? (
                      <div className="absolute inset-y-[-2px] w-0.5 bg-foreground" style={{ left: `${barGeometry(today, today, start, end).left}%` }} />
                    ) : null}
                  </div>
                </li>
              );
            })}
          </ul>
        </>
      )}
    </section>
  );
}
