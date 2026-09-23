'use client';

import { useTranslations } from 'next-intl';
import { useEffect, useMemo, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Combobox } from '@/components/ui/combobox';
import { DateRangePicker, type DateRange } from '@/components/ui/date-range-picker';
import { Dialog } from '@/components/ui/dialog';
import { NumberInput } from '@/components/ui/number-input';
import { Select } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import type { CarbonActivity, CarbonCatalogueMain, EmissionFactorView } from '@/lib/api/types';
import { formatNumber } from '@/lib/format';

import { factorOptions, pathDetail, unitOptions } from './factor-options';
import { isoKey, scopeKey, subKey } from './labels';

/** The body the dialog produces; the container adds the building (R305). */
export type ActivityEntry = {
  sub_category: string;
  factor_key: string;
  unit: string;
  quantity: string;
  period_start: string;
  period_end: string;
  description?: string;
  details: Record<string, string>;
};

export type ActivityDialogProps = {
  open: boolean;
  sub: CarbonCatalogueMain['subs'][number];
  factors: EmissionFactorView[];
  initial?: CarbonActivity;
  /** The latest allowed period end: today in Istanbul (R305). */
  today: string;
  errors: Record<string, string>;
  saving: boolean;
  onSubmit: (entry: ActivityEntry) => void;
  onClose: () => void;
};

/** R324's entry modal: period, emission source, unit, quantity, description; the emission is the server's. */
export function ActivityDialog({ open, sub, factors, initial, today, errors, saving, onSubmit, onClose }: ActivityDialogProps) {
  const t = useTranslations('carbon');
  const [range, setRange] = useState<DateRange | null>(null);
  const [factorKey, setFactorKey] = useState<string | null>(null);
  const [unit, setUnit] = useState<string | null>(null);
  const [quantity, setQuantity] = useState<string | null>(null);
  const [description, setDescription] = useState('');

  useEffect(() => {
    if (!open) return;
    setRange(initial ? { from: initial.period_start, to: initial.period_end } : null);
    setFactorKey(initial?.factor_key ?? null);
    setUnit(initial?.unit ?? null);
    setQuantity(initial?.quantity ?? null);
    setDescription(initial?.description ?? '');
  }, [open, initial]);

  const options = useMemo(() => factorOptions(factors), [factors]);
  const chosen = factors.find((f) => f.key === factorKey);
  const units = chosen ? unitOptions(chosen) : [];
  const valid = Boolean(range && chosen && unit && units.some((u) => u.value === unit) && quantity && Number(quantity) > 0);

  const submit = () => {
    if (!range || !chosen || !unit || !quantity) return;
    onSubmit({
      sub_category: sub.key,
      factor_key: chosen.key,
      unit,
      quantity,
      period_start: range.from,
      period_end: range.to,
      ...(description.trim() ? { description: description.trim() } : {}),
      details: pathDetail(chosen),
    });
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t(initial ? 'dialog.editTitle' : 'dialog.addTitle', { sub: t(subKey(sub.key)) })}
      size="lg"
      footer={
        <Button loading={saving} disabled={!valid} onClick={submit}>
          {t('dialog.save')}
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        <p className="text-foreground-muted type-small">
          {t('dialog.mapping', { scope: t(scopeKey(sub.scope)), iso: t(isoKey(sub.iso_category)) })}
        </p>
        <DateRangePicker label={t('dialog.period')} value={range} onValueChange={setRange} max={today} required
          error={errors.period_start ?? errors.period_end} />
        {options.length === 0 ? (
          <p className="text-foreground type-body">{t('dialog.factorEmpty')}</p>
        ) : (
          <Combobox
            label={t('dialog.factor')}
            options={options}
            value={factorKey}
            onValueChange={(v) => {
              setFactorKey(v);
              const next = factors.find((f) => f.key === v);
              setUnit(next ? next.base_unit : null);
            }}
            searchPlaceholder={t('dialog.factorSearch')}
            emptyText={t('dialog.factorEmpty')}
            error={errors.factor_key}
            required
          />
        )}
        {chosen ? (
          <div className="flex flex-col gap-1 rounded-md border border-border bg-surface-sunken p-3">
            <p className="text-foreground type-small">
              {t('dialog.factorInfo', { value: formatNumber(chosen.base_factor), unit: chosen.base_unit })}
            </p>
            {chosen.source ? (
              <p className="text-foreground-muted type-small">
                {t('dialog.factorSource', { source: [chosen.source, chosen.source_year].filter(Boolean).join(', ') })}
              </p>
            ) : null}
          </div>
        ) : null}
        <div className="grid gap-4 sm:grid-cols-2">
          <NumberInput label={t('dialog.quantity')} value={quantity} onValueChange={setQuantity} min="0" required error={errors.quantity} />
          <Select label={t('dialog.unit')} value={unit ?? ''} onValueChange={setUnit} options={units} disabled={!chosen} error={errors.unit} />
        </div>
        <Textarea label={t('dialog.description')} value={description} maxLength={500} rows={3}
          onChange={(e) => setDescription(e.target.value)} error={errors.description} />
        <p className="text-foreground-muted type-small">{t('dialog.computed')}</p>
      </div>
    </Dialog>
  );
}
