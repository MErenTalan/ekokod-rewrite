'use client';

import { useLocale, useTranslations } from 'next-intl';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { EmptyState } from '@/components/ui/empty-state';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Locale } from '@/i18n/locale';
import { formatDate } from '@/lib/format';
import type { NationalTariff } from '@/lib/api/types';

import { optionKey } from './tariff-labels';

import type { NationalTariffFields } from '@/lib/api/types';

export type NationalTariffDraft = NationalTariffFields;

export type DefaultsTabViewProps = {
  entries: NationalTariff[];
  onPublish: (draft: NationalTariffDraft) => void;
  onDelete: (id: string) => void;
  loading?: boolean;
};

const USER_GROUPS = [
  'residential', 'residential_plus', 'commercial', 'commercial_plus', 'industrial',
  'agricultural', 'lighting', 'martyrs_families', 'public_lighting',
] as const;
const VOLTAGE_LEVELS = ['lv', 'mv'] as const;
const TERMS = ['monomial', 'binomial'] as const;

const emptyDraft = (): NationalTariffDraft => ({
  effective_from: '', user_group: 'commercial', voltage_level: 'lv', term: 'monomial',
  energy_price: '', distribution_price: '', vat_rate: '',
});

/**
 * The admin tab of 01 §7.11. These rows ARE the national tariff schedule
 * (R243): the same catalogue the public bill calculator will read, which is
 * why publishing the same key edits the row rather than adding another.
 */
export function DefaultsTabView({ entries, onPublish, onDelete, loading = false }: DefaultsTabViewProps) {
  const t = useTranslations('tariffs');
  const common = useTranslations('common');
  const locale = useLocale() as Locale;
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<NationalTariffDraft>(emptyDraft());

  const submit = () => {
    if (!draft.effective_from || !draft.energy_price || !draft.distribution_price || !draft.vat_rate) return;
    onPublish(draft);
    setOpen(false);
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h3 className="type-h3">{t('defaults.title')}</h3>
          <p className="text-foreground-muted type-caption">{t('defaults.description')}</p>
        </div>
        <Button
          onClick={() => {
            setDraft(emptyDraft());
            setOpen(true);
          }}
        >
          {t('defaults.new')}
        </Button>
      </div>

      <p className="text-foreground-muted type-caption">{t('defaults.upsertNote')}</p>

      {loading ? (
        <Skeleton className="h-48 w-full" />
      ) : entries.length === 0 ? (
        <EmptyState title={t('defaults.empty')} description={t('defaults.emptyDescription')} />
      ) : (
        <TableContainer label={t('defaults.title')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('columns.effectiveFrom')}</TableHead>
                <TableHead>{t('fields.userGroup')}</TableHead>
                <TableHead>{t('fields.voltageLevel')}</TableHead>
                <TableHead>{t('fields.term')}</TableHead>
                <TableHead>{t('defaults.energyPrice')}</TableHead>
                <TableHead>{t('defaults.distributionPrice')}</TableHead>
                <TableHead>{t('fields.vatRate')}</TableHead>
                <TableHead>{t('defaults.source')}</TableHead>
                <TableHead>{common('actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries.map((entry) => (
                <TableRow key={entry.id}>
                  <TableCell className="font-medium">{formatDate(entry.effective_from, locale)}</TableCell>
                  <TableCell>{t(`options.userGroup.${optionKey(entry.user_group)}` as never)}</TableCell>
                  <TableCell>{t(`options.voltageLevel.${optionKey(entry.voltage_level)}` as never)}</TableCell>
                  <TableCell>{t(`options.term.${optionKey(entry.term)}` as never)}</TableCell>
                  <TableCell className="type-data">{entry.energy_price}</TableCell>
                  <TableCell className="type-data">{entry.distribution_price}</TableCell>
                  <TableCell className="type-data">{entry.vat_rate}</TableCell>
                  <TableCell>{entry.source ?? '—'}</TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" onClick={() => onDelete(entry.id)}>{common('delete')}</Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t('defaults.new')}
        size="lg"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>{common('cancel')}</Button>
            <Button onClick={submit}>{common('save')}</Button>
          </>
        }
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <Input
            label={t('fields.effectiveFrom')}
            required
            type="date"
            value={draft.effective_from}
            onChange={(e) => setDraft({ ...draft, effective_from: e.target.value })}
          />
          <Select
            label={t('fields.userGroup')}
            required
            value={draft.user_group}
            options={USER_GROUPS.map((g) => ({ value: g, label: t(`options.userGroup.${optionKey(g)}` as never) }))}
            onValueChange={(user_group) => setDraft({ ...draft, user_group: user_group as NationalTariffDraft['user_group'] })}
          />
          <Select
            label={t('fields.voltageLevel')}
            required
            value={draft.voltage_level}
            options={VOLTAGE_LEVELS.map((v) => ({ value: v, label: t(`options.voltageLevel.${optionKey(v)}` as never) }))}
            onValueChange={(voltage_level) => setDraft({ ...draft, voltage_level: voltage_level as NationalTariffDraft['voltage_level'] })}
          />
          <Select
            label={t('fields.term')}
            required
            value={draft.term}
            options={TERMS.map((term) => ({ value: term, label: t(`options.term.${optionKey(term)}` as never) }))}
            onValueChange={(term) => setDraft({ ...draft, term: term as NationalTariffDraft['term'] })}
          />
          <Input
            label={t('defaults.energyPrice')}
            required
            inputMode="decimal"
            value={draft.energy_price}
            onChange={(e) => setDraft({ ...draft, energy_price: e.target.value })}
          />
          <Input
            label={t('defaults.distributionPrice')}
            required
            inputMode="decimal"
            value={draft.distribution_price}
            onChange={(e) => setDraft({ ...draft, distribution_price: e.target.value })}
          />
          <Input
            label={t('fields.vatRate')}
            required
            inputMode="decimal"
            value={draft.vat_rate}
            onChange={(e) => setDraft({ ...draft, vat_rate: e.target.value })}
          />
          <Input
            label={t('defaults.powerPrice')}
            inputMode="decimal"
            value={draft.power_price ?? ''}
            onChange={(e) => setDraft({ ...draft, power_price: e.target.value })}
          />
          <Input
            label={t('defaults.overusePrice')}
            inputMode="decimal"
            value={draft.overuse_price ?? ''}
            onChange={(e) => setDraft({ ...draft, overuse_price: e.target.value })}
          />
          <Input
            label={t('defaults.dailyThreshold')}
            inputMode="decimal"
            value={draft.daily_threshold_kwh ?? ''}
            onChange={(e) => setDraft({ ...draft, daily_threshold_kwh: e.target.value })}
          />
          <Input
            label={t('defaults.source')}
            value={draft.source ?? ''}
            onChange={(e) => setDraft({ ...draft, source: e.target.value })}
          />
        </div>
      </Dialog>
    </div>
  );
}
