'use client';

import { useLocale, useTranslations } from 'next-intl';
import type { FormEvent } from 'react';

import { Button } from '@/components/ui/button';
import { DatePicker } from '@/components/ui/date-picker';
import { NumberInput } from '@/components/ui/number-input';
import { RadioGroup } from '@/components/ui/radio-group';
import { Select } from '@/components/ui/select';
import type { components } from '@/lib/api/schema';
import { formatCurrency, formatDate, formatNumber } from '@/lib/format';
import type { Locale } from '@/i18n/locale';

import type { CalcField, CalcGroup, CalcValues } from './calculator-form';

export type PublicBill = components['schemas']['PublicBill'];
export type BillCalculatorProps = {
  values: CalcValues;
  errors: Partial<Record<CalcField, string>>;
  result: PublicBill | null;
  pending: boolean;
  onChange: (patch: Partial<CalcValues>) => void;
  onSubmit: () => void;
};

const GROUPS: CalcGroup[] = ['residential', 'commercial', 'industrial', 'agricultural', 'lighting'];
const LINES = ['energy', 'distribution', 'power', 'overuse', 'vatBase'] as const;
const FIELD_OF = { energy: 'energy', distribution: 'distribution', power: 'power', overuse: 'overuse', vatBase: 'vat_base' } as const;

/** 01 §7.19 public bill calculator: every charge line including power (Q-H3) and the total/T rule (Q-H4). */
export function BillCalculator({ values: v, errors, result, pending, onChange, onSubmit }: BillCalculatorProps) {
  const t = useTranslations('site.calculator');
  const locale = useLocale() as Locale;
  const submit = (event: FormEvent) => {
    event.preventDefault();
    onSubmit();
  };
  const num = (field: 't1' | 't2' | 't3' | 'total' | 'demand' | 'contract', label: string, description?: string) => (
    <NumberInput label={label} description={description} value={v[field]} error={errors[field]} min="0" fractionDigits={3} onValueChange={(value) => onChange({ [field]: value })} />
  );
  const base = result?.tariff.group_used.replace(/_plus$/, '') as CalcGroup | undefined;
  return (
    <div className="mx-auto grid max-w-7xl gap-8 px-4 py-12 sm:px-6 lg:grid-cols-[1fr_minmax(320px,400px)]">
      <form noValidate onSubmit={submit} aria-label={t('title')} className="flex flex-col gap-6">
        <div className="grid gap-4 sm:grid-cols-2">
          <Select label={t('group')} value={v.group} error={errors.group} options={GROUPS.map((g) => ({ value: g, label: t(`groups.${g}`) }))} onValueChange={(g) => onChange({ group: g as CalcGroup })} />
          <RadioGroup label={t('voltage')} value={v.voltage} error={errors.voltage} orientation="horizontal"
            options={[{ value: 'lv', label: t('voltages.lv') }, { value: 'mv', label: t('voltages.mv') }]}
            onValueChange={(value) => onChange({ voltage: value as CalcValues['voltage'] })} />
          <RadioGroup label={t('term')} description={t('binomialHint')} value={v.term} error={errors.term} orientation="horizontal"
            options={[{ value: 'monomial', label: t('terms.monomial') }, { value: 'binomial', label: t('terms.binomial'), disabled: v.voltage !== 'mv' }]}
            onValueChange={(value) => onChange({ term: value as CalcValues['term'] })} />
          <RadioGroup label={t('tariff')} value={v.multi ? 'multi' : 'single'} error={errors.multi} orientation="horizontal"
            options={[{ value: 'single', label: t('tariffs.single') }, { value: 'multi', label: t('tariffs.multi'), disabled: v.group === 'lighting' }]}
            onValueChange={(value) => onChange({ multi: value === 'multi' })} />
          <DatePicker label={t('start')} required value={v.start} error={errors.start} onValueChange={(start) => onChange({ start })} />
          <DatePicker label={t('end')} required value={v.end} error={errors.end} min={v.start ?? undefined} onValueChange={(end) => onChange({ end })} />
        </div>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {num('total', t('total'))}
          {num('t1', t('t1'))}
          {num('t2', t('t2'))}
          {num('t3', t('t3'))}
        </div>
        {v.term === 'binomial' ? (
          <div className="grid gap-4 sm:grid-cols-2">
            {num('contract', t('contract'))}
            {num('demand', t('demand'))}
          </div>
        ) : null}
        <Button type="submit" size="lg" loading={pending} className="self-start">{t('calculate')}</Button>
        <section aria-labelledby="calc-rules" className="flex flex-col gap-2 rounded-lg border border-border bg-surface-sunken p-5">
          <h2 id="calc-rules" className="text-foreground type-h3">{t('rulesTitle')}</h2>
          <ul className="flex list-disc flex-col gap-2 ps-5 text-foreground-muted">
            <li>{t('rules.bands')}</li>
            <li>{t('rules.power')}</li>
            <li>{t('rules.vat')}</li>
          </ul>
        </section>
      </form>

      <section aria-label={t('result')} aria-live="polite" className="flex h-fit flex-col gap-4 rounded-lg border border-border bg-surface p-6 lg:sticky lg:top-24">
        <h2 className="text-foreground type-h2">{t('result')}</h2>
        {result && base ? (
          <>
            <dl className="flex flex-col gap-2">
              {LINES.map((line) => (
                <div key={line} className="flex items-baseline justify-between gap-4 border-b border-border pb-2">
                  <dt className="text-foreground-muted">{t(`lines.${line}`)}</dt>
                  <dd className="text-foreground type-data">{formatCurrency(result[FIELD_OF[line]])}</dd>
                </div>
              ))}
              <div className="flex items-baseline justify-between gap-4 border-b border-border pb-2">
                <dt className="text-foreground-muted">{t('lines.vat', { rate: formatNumber(result.vat_rate, { maxFractionDigits: 2 }) })}</dt>
                <dd className="text-foreground type-data">{formatCurrency(result.vat)}</dd>
              </div>
              <div className="flex items-baseline justify-between gap-4 pt-1">
                <dt className="font-semibold text-foreground">{t('lines.total')}</dt>
                <dd className="text-foreground type-metric">{formatCurrency(result.total)}</dd>
              </div>
            </dl>
            <ul className="flex flex-col gap-1 text-foreground-muted type-small">
              <li>{t('days', { days: result.days })}</li>
              <li>{t(`basis.${result.basis}`)}</li>
              <li>{t('tariffUsed', { effective: formatDate(result.tariff.effective_from, locale), group: t(`groups.${base}`) })}</li>
              {result.tariff.group_used !== base ? <li>{t('plusGroup')}</li> : null}
            </ul>
          </>
        ) : (
          <p className="text-foreground-muted">{t('resultEmpty')}</p>
        )}
      </section>
    </div>
  );
}
