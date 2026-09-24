'use client';

import { FileSpreadsheet, FileText, Mail } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { StatusBadge, type Status } from '@/components/ui/status-badge';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';

export type JobState = 'queued' | 'running' | 'succeeded' | 'failed';

export type ActionRow = {
  buildingId: string;
  buildingName: string;
  reportId: string;
  status: JobState;
  errorCode?: string;
};

export type ReportActionsViewProps = {
  canGenerate: boolean;
  canEmail: boolean;
  rows: ActionRow[];
  generating: boolean;
  onGenerate: () => void;
  onDownload: (reportId: string, format: 'pdf' | 'excel') => void;
  onEmail: (reportId: string) => void;
};

const TONE: Record<JobState, Status> = { queued: 'neutral', running: 'info', succeeded: 'success', failed: 'danger' };
const LABEL = { queued: 'queued', running: 'running', succeeded: 'ready', failed: 'failed' } as const;

/**
 * E-3: one report per building. Generation answers a job per building; each
 * row offers its files and e-mail once that building's report is ready.
 */
export function ReportActionsView({ canGenerate, canEmail, rows, generating, onGenerate, onDownload, onEmail }: ReportActionsViewProps) {
  const t = useTranslations('reports.actions');
  return (
    <Card className="p-4 flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="type-h3">{t('title')}</h2>
          <p className="text-foreground-muted type-small">{t('description')}</p>
        </div>
        {canGenerate ? (
          <Button onClick={onGenerate} loading={generating}>
            {generating ? t('generating') : t('generate')}
          </Button>
        ) : null}
      </div>
      {rows.length > 0 ? (
        <TableContainer label={t('title')}>
          <Table aria-label={t('title')}>
            <TableHeader>
              <TableRow>
                <TableHead>{t('building')}</TableHead>
                <TableHead>{t('status')}</TableHead>
                <TableHead><span className="sr-only">{t('title')}</span></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => (
                <TableRow key={row.buildingId}>
                  <TableHead scope="row">{row.buildingName}</TableHead>
                  <TableCell>
                    <div className="flex flex-col gap-1" aria-live="polite">
                      <StatusBadge status={TONE[row.status]} label={t(LABEL[row.status])} />
                      {row.status === 'failed' ? (
                        <span className="text-danger type-small">
                          {row.errorCode === 'report_building_not_found' ? t('errors.buildingNotFound') : t('errors.generic')}
                        </span>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell>
                    {row.status === 'succeeded' ? (
                      <div className="flex flex-wrap gap-2">
                        <Button variant="secondary" size="sm" iconStart={FileText} onClick={() => onDownload(row.reportId, 'pdf')}>
                          {t('pdf')}
                        </Button>
                        <Button variant="secondary" size="sm" iconStart={FileSpreadsheet} onClick={() => onDownload(row.reportId, 'excel')}>
                          {t('excel')}
                        </Button>
                        {canEmail ? (
                          <Button variant="secondary" size="sm" iconStart={Mail} onClick={() => onEmail(row.reportId)}>
                            {t('email')}
                          </Button>
                        ) : null}
                      </div>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      ) : null}
    </Card>
  );
}
