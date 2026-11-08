'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { DatePicker } from '@/components/ui/date-picker';
import { Dialog } from '@/components/ui/dialog';
import { Select } from '@/components/ui/select';
import {
  Table, TableBody, TableCell, TableContainer, TableFooter, TableHead, TableHeader, TableRow,
} from '@/components/ui/table';
import { formatCurrency, formatNumber } from '@/lib/format';
import type { Analyzer, BillPeriod, BillPreview } from '@/lib/api/types';

import { formatBound, HOURS, keyOptions, splitInstant, toInstant } from './custom-period';
import { dayRanges } from './missing-days';

export type PreviewRequest = { scope: 'analyzer' | 'building'; subjectId: string; from: string; to: string };
export type SaveRequest = { periodKey: string; from: string; to: string };

export type CustomPeriodCardViewProps = {
  analyzers: Analyzer[];
  /** The selected building: required for a building bill and for saving a period. */
  buildingId: string | null;
  savedPeriods?: BillPeriod[];
  preview?: BillPreview;
  /** API field errors by name: from, to. */
  errors?: Record<string, string>;
  previewing?: boolean;
  saving?: boolean;
  onPreview: (request: PreviewRequest) => void;
  onSave: (request: SaveRequest) => void;
  /** `done` closes the confirmation; it is called only after a successful delete. */
  onDelete: (periodKey: string, done: () => void) => void;
};

/**
 * F30: a bill over any date range (the utility reads meters on its own days,
 * often mid-afternoon), previewed without saving, then optionally saved as the
 * official window of a period key so generated bills follow it (R554–R559).
 */
export function CustomPeriodCardView({
  analyzers, buildingId, savedPeriods = [], preview, errors = {}, previewing = false, saving = false,
  onPreview, onSave, onDelete,
}: CustomPeriodCardViewProps) {
  const t = useTranslations('bills.custom');
  const [subject, setSubject] = useState<string>(buildingId ? `building:${buildingId}` : '');
  const [fromDate, setFromDate] = useState<string | null>(null);
  const [fromHour, setFromHour] = useState('00:00');
  const [toDate, setToDate] = useState<string | null>(null);
  const [toHour, setToHour] = useState('00:00');
  const [key, setKey] = useState<string | null>(null);
  const [missing, setMissing] = useState<Record<string, string>>({});
  const [deleting, setDeleting] = useState<string | null>(null);

  const subjects = [
    ...(buildingId ? [{ value: `building:${buildingId}`, label: t('buildingSubject') }] : []),
    ...analyzers.map((a) => ({
      value: `analyzer:${a.id}`,
      label: [a.installation_number, a.customer_name].filter(Boolean).join(' · ') || a.id,
    })),
  ];
  const keys = fromDate ? keyOptions(fromDate) : [];
  const chosenKey = key && keys.includes(key) ? key : (keys[1] ?? null);

  const window = () => {
    const absent: Record<string, string> = {};
    if (!fromDate) absent.from = t('required');
    if (!toDate) absent.to = t('required');
    setMissing(absent);
    if (!fromDate || !toDate) return null;
    return { from: toInstant(fromDate, fromHour), to: toInstant(toDate, toHour) };
  };

  const previewClicked = () => {
    const w = window();
    if (!subject) setMissing((m) => ({ ...m, subject: t('required') }));
    if (!w || !subject) return;
    const [scope, subjectId] = subject.split(':') as ['analyzer' | 'building', string];
    onPreview({ scope, subjectId, ...w });
  };

  const saveClicked = () => {
    const w = window();
    if (!w || !chosenKey) return;
    onSave({ periodKey: chosenKey, ...w });
  };

  const load = (p: BillPeriod) => {
    const from = splitInstant(p.period_start);
    const to = splitInstant(p.period_end);
    setFromDate(from.date); setFromHour(from.hour); setToDate(to.date); setToHour(to.hour); setKey(p.period_key);
  };

  const error = (field: string) => missing[field] ?? errors[field];
  const bill = preview?.bill;

  return (
    <Card className="p-4 flex flex-col gap-4">
      <div>
        <h3 className="type-h3">{t('title')}</h3>
        <p className="text-foreground-muted type-caption">{t('description')}</p>
      </div>

      <Select label={t('subject')} value={subject} onValueChange={setSubject} options={subjects}
        placeholder={t('subjectPlaceholder')} error={error('subject')} />

      <div className="grid gap-3 sm:grid-cols-4">
        <DatePicker label={t('from')} value={fromDate} onValueChange={setFromDate} error={error('from')} required />
        <Select label={t('fromHour')} value={fromHour} onValueChange={setFromHour} options={HOURS.map((h) => ({ value: h, label: h }))} />
        <DatePicker label={t('to')} value={toDate} onValueChange={setToDate} error={error('to')} required />
        <Select label={t('toHour')} value={toHour} onValueChange={setToHour} options={HOURS.map((h) => ({ value: h, label: h }))} />
      </div>
      <p className="text-foreground-muted type-caption">{t('hourHint')}</p>

      <div className="flex flex-wrap items-end gap-2">
        <Button loading={previewing} onClick={previewClicked}>{t('preview')}</Button>
        {buildingId && keys.length > 0 ? (
          <>
            <div className="w-40">
              <Select label={t('periodKey')} value={chosenKey ?? ''} onValueChange={setKey}
                options={keys.map((k) => ({ value: k, label: k }))} error={errors.period} />
            </div>
            <Button variant="secondary" loading={saving} onClick={saveClicked}>{t('save')}</Button>
          </>
        ) : null}
      </div>
      {!buildingId ? <p className="text-foreground-muted type-caption">{t('saveNeedsBuilding')}</p> : null}

      {bill ? (
        <div className="flex flex-col gap-3">
          <p className="type-body text-foreground">
            {t('previewHeader', {
              from: formatBound(bill.period_start), to: formatBound(bill.period_end), days: bill.days_in_period,
              key: bill.period_key,
            })}
          </p>
          {bill.missing_days.length > 0 ? (
            <Alert tone="warning" title={t('partialTitle')}>
              {t('partialDays', { days: dayRanges(bill.missing_days).join(', ') })}
            </Alert>
          ) : null}
          {bill.flag_reason === 'ptf_data_missing' ? <Alert tone="warning" title={t('ptfMissing')} /> : null}
          <TableContainer label={t('linesLabel')}>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('line')}</TableHead>
                  <TableHead numeric>{t('quantity')}</TableHead>
                  <TableHead numeric>{t('unitPrice')}</TableHead>
                  <TableHead numeric>{t('amount')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(preview?.lines ?? []).map((l) => (
                  <TableRow key={l.code + l.label}>
                    <TableCell>{l.label}</TableCell>
                    <TableCell numeric>
                      {l.rate_pct ? `%${formatNumber(l.rate_pct)}` : l.quantity ? `${formatNumber(l.quantity, { maxFractionDigits: 3 })} ${l.unit ?? ''}` : '—'}
                    </TableCell>
                    <TableCell numeric>{l.unit_price ? formatNumber(l.unit_price, { maxFractionDigits: 6 }) : '—'}</TableCell>
                    <TableCell numeric>{formatCurrency(l.amount)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
              <TableFooter>
                <TableRow>
                  <TableCell>{t('total')}</TableCell>
                  <TableCell numeric>{formatNumber(bill.active_import, { maxFractionDigits: 3 })} kWh</TableCell>
                  <TableCell />
                  <TableCell numeric>{formatCurrency(bill.total_cost)}</TableCell>
                </TableRow>
              </TableFooter>
            </Table>
          </TableContainer>
        </div>
      ) : null}

      {savedPeriods.length > 0 ? (
        <div className="flex flex-col gap-2">
          <h4 className="type-label">{t('savedTitle')}</h4>
          <ul className="flex flex-col gap-1">
            {savedPeriods.map((p) => (
              <li key={p.period_key} className="flex flex-wrap items-center gap-3 type-body">
                <span className="font-medium">{p.period_key}</span>
                <span>{formatBound(p.period_start)} – {formatBound(p.period_end)}</span>
                <Button variant="ghost" onClick={() => load(p)}>{t('edit')}</Button>
                <Button variant="ghost" onClick={() => setDeleting(p.period_key)}>{t('delete')}</Button>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => { if (!open) setDeleting(null); }}
        title={t('deleteTitle', { key: deleting ?? '' })}
        footer={
          <Button variant="danger" loading={saving} onClick={() => deleting && onDelete(deleting, () => setDeleting(null))}>
            {t('delete')}
          </Button>
        }
      >
        <p className="type-body">{t('deleteDescription')}</p>
      </Dialog>
    </Card>
  );
}
