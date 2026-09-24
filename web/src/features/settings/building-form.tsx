'use client';

import { Trash2 } from 'lucide-react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { IconButton } from '@/components/ui/icon-button';
import { Input } from '@/components/ui/input';
import { NumberInput } from '@/components/ui/number-input';
import { Select } from '@/components/ui/select';
import { Table, TableBody, TableCaption, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Textarea } from '@/components/ui/textarea';
import type { BuildingCreateRequest, BuildingDetail, TariffSummary, User } from '@/lib/api/types';
import { formatDate } from '@/lib/format';
import type { Locale } from '@/i18n/locale';
import { useLocale } from 'next-intl';

export type BuildingDraft = {
  id?: string;
  name: string;
  address: string;
  latitude: string;
  longitude: string;
  floors: string;
  personnelCount: string;
  totalAreaM2: string;
  responsibleUserId: string | null;
  sector: string;
  billCutoffDay: string;
  contacts: { name: string; phone: string }[];
};

export const MAX_CONTACTS = 10;

export const emptyBuilding = (): BuildingDraft => ({
  name: '',
  address: '',
  latitude: '',
  longitude: '',
  floors: '',
  personnelCount: '',
  totalAreaM2: '',
  responsibleUserId: null,
  sector: '',
  billCutoffDay: '1',
  contacts: [],
});

export const buildingDraft = (building: BuildingDetail): BuildingDraft => ({
  id: building.id,
  name: building.name,
  address: building.address ?? '',
  latitude: building.latitude ?? '',
  longitude: building.longitude ?? '',
  floors: building.floors == null ? '' : String(building.floors),
  personnelCount: building.personnel_count == null ? '' : String(building.personnel_count),
  totalAreaM2: building.total_area_m2 ?? '',
  responsibleUserId: building.responsible_user_id ?? null,
  sector: building.sector ?? '',
  billCutoffDay: String(building.bill_cutoff_day),
  contacts: building.contacts.map((c) => ({ name: c.name ?? '', phone: c.phone ?? '' })),
});

/** The request for a building (R175): a cleared responsible user says so explicitly. */
export function toBuildingRequest(draft: BuildingDraft): BuildingCreateRequest & { clear_responsible_user?: boolean } {
  return {
    name: draft.name,
    address: draft.address || null,
    sector: draft.sector || null,
    // A decimal the user left blank is omitted entirely; the API has no null for one.
    ...(draft.latitude ? { latitude: draft.latitude } : {}),
    ...(draft.longitude ? { longitude: draft.longitude } : {}),
    ...(draft.totalAreaM2 ? { total_area_m2: draft.totalAreaM2 } : {}),
    floors: draft.floors === '' ? null : Number(draft.floors),
    personnel_count: draft.personnelCount === '' ? null : Number(draft.personnelCount),
    bill_cutoff_day: Number(draft.billCutoffDay),
    contacts: draft.contacts.filter((c) => c.name || c.phone),
    ...(draft.responsibleUserId ? { responsible_user_id: draft.responsibleUserId } : { clear_responsible_user: true }),
  };
}

export type BuildingFormViewProps = {
  value: BuildingDraft;
  onChange: (draft: BuildingDraft) => void;
  users: User[];
  tariffHistory: TariffSummary[];
  fieldErrors?: Record<string, string>;
};

/** Building fields of 01 §7.15; the tariff history is read-only here (F8 edits it). */
export function BuildingFormView({ value, onChange, users, tariffHistory, fieldErrors = {} }: BuildingFormViewProps) {
  const t = useTranslations('settings.buildings');
  const locale = useLocale() as Locale;
  const set = (patch: Partial<BuildingDraft>) => onChange({ ...value, ...patch });
  const cutoff = Number(value.billCutoffDay);
  const cutoffError = value.billCutoffDay !== '' && (!Number.isInteger(cutoff) || cutoff < 1 || cutoff > 31) ? t('cutoffRange') : undefined;

  return (
    <div className="flex flex-col gap-4">
      <Input label={t('name')} value={value.name} error={fieldErrors.name} onChange={(e) => set({ name: e.target.value })} required />
      <Textarea label={t('address')} value={value.address} rows={2} onChange={(e) => set({ address: e.target.value })} />
      <div className="flex flex-wrap gap-4">
        <NumberInput label={t('latitude')} value={value.latitude} onValueChange={(v) => set({ latitude: v ?? '' })} fractionDigits={6} />
        <NumberInput label={t('longitude')} value={value.longitude} onValueChange={(v) => set({ longitude: v ?? '' })} fractionDigits={6} />
      </div>
      <div className="flex flex-wrap gap-4">
        <NumberInput label={t('floors')} value={value.floors} onValueChange={(v) => set({ floors: v ?? '' })} fractionDigits={0} />
        <NumberInput label={t('personnel')} value={value.personnelCount} onValueChange={(v) => set({ personnelCount: v ?? '' })} fractionDigits={0} />
        <NumberInput label={t('area')} value={value.totalAreaM2} onValueChange={(v) => set({ totalAreaM2: v ?? '' })} fractionDigits={2} />
      </div>
      <Input label={t('sector')} value={value.sector} error={fieldErrors.sector} onChange={(e) => set({ sector: e.target.value })} />
      <NumberInput
        label={t('cutoffDay')}
        value={value.billCutoffDay}
        error={fieldErrors.bill_cutoff_day ?? cutoffError}
        onValueChange={(v) => set({ billCutoffDay: v ?? '' })}
        min="1"
        max="31"
        fractionDigits={0}
      />
      <Select
        label={t('responsible')}
        options={users.map((user) => ({ value: user.id, label: `${user.name} · ${user.email}` }))}
        value={value.responsibleUserId}
        error={fieldErrors.responsible_user_id}
        placeholder={t('noResponsible')}
        onValueChange={(id) => set({ responsibleUserId: id })}
      />

      <fieldset className="flex flex-col gap-3">
        <legend className="text-foreground type-small font-semibold">{t('contacts')}</legend>
        {value.contacts.map((contact, index) => (
          <div key={index} className="flex flex-wrap items-end gap-2">
            <Input
              label={t('contactName')}
              value={contact.name}
              onChange={(e) => set({ contacts: value.contacts.map((c, i) => (i === index ? { ...c, name: e.target.value } : c)) })}
            />
            <Input
              label={t('contactPhone')}
              value={contact.phone}
              onChange={(e) => set({ contacts: value.contacts.map((c, i) => (i === index ? { ...c, phone: e.target.value } : c)) })}
            />
            <IconButton
              label={t('removeContact', { index: index + 1 })}
              icon={Trash2}
              size="sm"
              onClick={() => set({ contacts: value.contacts.filter((_, i) => i !== index) })}
            />
          </div>
        ))}
        <Button
          variant="secondary"
          size="sm"
          className="self-start"
          disabled={value.contacts.length >= MAX_CONTACTS}
          onClick={() => set({ contacts: [...value.contacts, { name: '', phone: '' }] })}
        >
          {t('addContact')}
        </Button>
      </fieldset>

      {value.id ? (
        <section className="flex flex-col gap-2">
          <h3 className="text-foreground type-h3">{t('tariffHistory')}</h3>
          {tariffHistory.length === 0 ? (
            <p className="text-foreground-muted type-small">{t('noTariff')}</p>
          ) : (
            <TableContainer label={t('tariffHistory')}>
              <Table>
                <TableCaption>{t('tariffHistory')}</TableCaption>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('effectiveFrom')}</TableHead>
                    <TableHead>{t('tariffName')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {tariffHistory.map((tariff) => (
                    <TableRow key={tariff.id}>
                      <TableCell>{formatDate(tariff.effective_from, locale)}</TableCell>
                      <TableCell>{tariff.name ?? '—'}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
          <Button asChild variant="secondary" size="sm" className="self-start">
            <Link href={`/ekorm/tariffs?building_id=${value.id}`}>{t('manageTariffs')}</Link>
          </Button>
        </section>
      ) : null}
    </div>
  );
}
