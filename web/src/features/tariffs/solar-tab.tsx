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
import type { SolarTariff } from '@/lib/api/types';

import { optionKey } from './tariff-labels';

import type { SolarTariffFields } from '@/lib/api/types';

export type SolarTariffDraft = SolarTariffFields & { currency: NonNullable<SolarTariffFields['currency']> };

export type SolarTabViewProps = {
  plants: { id: string; name: string }[];
  plantID: string | null;
  onPlantChange: (id: string) => void;
  tariffs: SolarTariff[];
  /** tariffs.edit (A/CA) gates the writes; the history is A/CA/CR. */
  canEdit: boolean;
  onCreate: (draft: SolarTariffDraft) => void;
  onDelete: (id: string) => void;
  loading?: boolean;
};

const CURRENCIES = ['TRY', 'USD', 'EUR'] as const;

/** The solar tab of 01 §7.11: a plant's feed-in and purchase prices. */
export function SolarTabView({
  plants, plantID, onPlantChange, tariffs, canEdit, onCreate, onDelete, loading = false,
}: SolarTabViewProps) {
  const t = useTranslations('tariffs');
  const common = useTranslations('common');
  const locale = useLocale() as Locale;
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<SolarTariffDraft>({
    plant_id: plantID ?? '', effective_from: '', feed_in_tariff: '', currency: 'TRY',
  });

  const startCreate = () => {
    setDraft({ plant_id: plantID ?? '', effective_from: '', feed_in_tariff: '', currency: 'TRY' });
    setOpen(true);
  };

  const submit = () => {
    if (!plantID || !draft.effective_from || !draft.feed_in_tariff) return;
    onCreate({ ...draft, plant_id: plantID });
    setOpen(false);
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-64">
          <h3 className="type-h3">{t('solar.title')}</h3>
          <p className="text-foreground-muted type-caption">{t('solar.description')}</p>
        </div>
        <Select
          label={t('solar.selectPlant')}
          value={plantID}
          placeholder={t('solar.selectPlant')}
          options={plants.map((p) => ({ value: p.id, label: p.name }))}
          onValueChange={onPlantChange}
        />
        {canEdit && plantID ? <Button onClick={startCreate}>{t('solar.new')}</Button> : null}
      </div>

      {!plantID ? (
        <EmptyState title={t('solar.selectPlant')} description={t('solar.plantRequired')} />
      ) : loading ? (
        <Skeleton className="h-48 w-full" />
      ) : tariffs.length === 0 ? (
        <EmptyState
          title={t('solar.empty')}
          description={t('solar.emptyDescription')}
          action={canEdit ? <Button onClick={startCreate}>{t('solar.addFirst')}</Button> : undefined}
        />
      ) : (
        <TableContainer label={t('solar.title')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('columns.effectiveFrom')}</TableHead>
                <TableHead>{t('solar.feedInTariff')}</TableHead>
                <TableHead>{t('solar.purchasePrice')}</TableHead>
                <TableHead>{t('fields.currency')}</TableHead>
                <TableHead>{t('solar.notes')}</TableHead>
                {canEdit ? <TableHead>{common('actions')}</TableHead> : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {tariffs.map((tariff) => (
                <TableRow key={tariff.id}>
                  <TableCell className="font-medium">{formatDate(tariff.effective_from, locale)}</TableCell>
                  <TableCell className="type-data">{tariff.feed_in_tariff}</TableCell>
                  <TableCell className="type-data">{tariff.purchase_price ?? '—'}</TableCell>
                  <TableCell>{t(`options.currency.${optionKey(tariff.currency ?? 'TRY')}` as never)}</TableCell>
                  <TableCell>{tariff.notes ?? '—'}</TableCell>
                  {canEdit ? (
                    <TableCell>
                      <Button variant="ghost" size="sm" onClick={() => onDelete(tariff.id)}>{common('delete')}</Button>
                    </TableCell>
                  ) : null}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t('solar.new')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>{common('cancel')}</Button>
            <Button onClick={submit}>{common('save')}</Button>
          </>
        }
      >
        <div className="flex flex-col gap-4">
          <Input
            label={t('fields.effectiveFrom')}
            required
            type="date"
            value={draft.effective_from}
            onChange={(e) => setDraft({ ...draft, effective_from: e.target.value })}
          />
          <Input
            label={t('solar.feedInTariff')}
            required
            inputMode="decimal"
            value={draft.feed_in_tariff}
            onChange={(e) => setDraft({ ...draft, feed_in_tariff: e.target.value })}
          />
          <Input
            label={t('solar.purchasePrice')}
            inputMode="decimal"
            value={draft.purchase_price ?? ''}
            onChange={(e) => setDraft({ ...draft, purchase_price: e.target.value })}
          />
          <Select
            label={t('fields.currency')}
            value={draft.currency}
            options={CURRENCIES.map((c) => ({ value: c, label: t(`options.currency.${c}` as never) }))}
            onValueChange={(currency) => setDraft({ ...draft, currency: currency as SolarTariffDraft['currency'] })}
          />
          <Input
            label={t('solar.notes')}
            value={draft.notes ?? ''}
            onChange={(e) => setDraft({ ...draft, notes: e.target.value })}
          />
        </div>
      </Dialog>
    </div>
  );
}
