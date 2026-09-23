'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { NumberInput } from '@/components/ui/number-input';
import type { EmissionFactorView } from '@/lib/api/types';

export type FactorOverride = { base_factor: string; source?: string; source_year?: number; source_url?: string };

export type FactorDialogProps = {
  factor: EmissionFactorView;
  saving: boolean;
  errors: Record<string, string>;
  onSubmit: (body: FactorOverride) => void;
  onClose: () => void;
};

/** R303: the company's own value for one factor, with where it comes from. */
export function FactorDialog({ factor, saving, errors, onSubmit, onClose }: FactorDialogProps) {
  const t = useTranslations('carbon');
  const [value, setValue] = useState<string | null>(factor.base_factor);
  const [source, setSource] = useState(factor.source ?? '');
  const [year, setYear] = useState(factor.source_year ? String(factor.source_year) : '');
  const [url, setUrl] = useState(factor.source_url ?? '');
  const yearNumber = Number(year);
  const yearValid = year === '' || (Number.isInteger(yearNumber) && yearNumber >= 1990 && yearNumber <= 2100);
  const valid = Boolean(value && Number(value) > 0 && yearValid);
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t('database.dialogTitle')}
      description={factor.label}
      footer={
        <Button
          loading={saving}
          disabled={!valid}
          onClick={() =>
            value &&
            onSubmit({
              base_factor: value,
              ...(source.trim() ? { source: source.trim() } : {}),
              ...(year ? { source_year: yearNumber } : {}),
              ...(url.trim() ? { source_url: url.trim() } : {}),
            })
          }
        >
          {t('database.save')}
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        <NumberInput label={t('database.value', { unit: factor.base_unit })} value={value} onValueChange={setValue} min="0" required error={errors.base_factor} />
        <Input label={t('database.sourceName')} value={source} maxLength={200} onChange={(e) => setSource(e.target.value)} error={errors.source} />
        <Input label={t('database.sourceYear')} value={year} inputMode="numeric" maxLength={4} onChange={(e) => setYear(e.target.value.replace(/\D/g, ''))}
          error={errors.source_year} />
        <Input label={t('database.sourceUrl')} value={url} maxLength={500} type="url" onChange={(e) => setUrl(e.target.value)} error={errors.source_url} />
      </div>
    </Dialog>
  );
}
