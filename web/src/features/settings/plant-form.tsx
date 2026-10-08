'use client';

import { Trash2 } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { DatePicker } from '@/components/ui/date-picker';
import { IconButton } from '@/components/ui/icon-button';
import { Input } from '@/components/ui/input';
import { NumberInput } from '@/components/ui/number-input';
import { Select } from '@/components/ui/select';
import { Table, TableBody, TableCaption, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Textarea } from '@/components/ui/textarea';
import type { Locale } from '@/i18n/locale';
import type { PlantCreateRequest, PlantDetail, PlantDevice, SolarTariff, SolarTariffFields } from '@/lib/api/types';
import { shiftDecimal } from '@/lib/decimal';
import { formatDate } from '@/lib/format';

export const MONTH_KEYS = [
  'january',
  'february',
  'march',
  'april',
  'may',
  'june',
  'july',
  'august',
  'september',
  'october',
  'november',
  'december',
] as const;

const ORIENTATIONS = ['n', 's', 'e', 'w', 'ne', 'se', 'nw', 'sw'] as const;
export const CURRENCIES = ['USD', 'EUR', 'TRY'] as const;
type Currency = (typeof CURRENCIES)[number];
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export type PlantDraft = {
  id?: string;
  name: string;
  installationNumber: string;
  pvBrandModel: string;
  panelPowerW: string;
  panelEfficiencyPct: string;
  panelCount: string;
  stringCount: string;
  orientation: (typeof ORIENTATIONS)[number] | '';
  tiltAngleDeg: string;
  totalCapacityKw: string;
  installationDate: string | null;
  address: string;
  latitude: string;
  longitude: string;
  yearlyTargetKwh: string;
  plantKind: 'rooftop' | 'grid';
  monthlyTargets: string[];
  alarmRecipients: string[];
  /** R289's optional netting analyzer; '' means none. */
  nettingAnalyzerId: string;
  /** What the server holds, so an edit that removes it can say so. */
  savedNettingAnalyzerId?: string;
  /** The sale price per MWh; saved as a solar tariff per kWh (legacy "Birim Fiyat"). */
  priceMwh: string;
  currency: Currency;
  /** Empty: the installation date for a new plant, today for a price change. */
  priceFrom: string | null;
  /** The plant's current solar tariff, so an unchanged price is not re-saved. */
  savedTariff?: SolarTariff;
};

export const emptyPlant = (): PlantDraft => ({
  name: '',
  installationNumber: '',
  pvBrandModel: '',
  panelPowerW: '',
  panelEfficiencyPct: '',
  panelCount: '',
  stringCount: '',
  orientation: '',
  tiltAngleDeg: '',
  totalCapacityKw: '',
  installationDate: null,
  address: '',
  latitude: '',
  longitude: '',
  yearlyTargetKwh: '',
  plantKind: 'rooftop',
  monthlyTargets: Array.from({ length: 12 }, () => ''),
  alarmRecipients: [],
  nettingAnalyzerId: '',
  priceMwh: '',
  currency: 'USD',
  priceFrom: null,
});

export const plantDraft = (plant: PlantDetail): PlantDraft => ({
  ...emptyPlant(),
  id: plant.id,
  name: plant.name,
  installationNumber: plant.installation_number ?? '',
  pvBrandModel: plant.pv_brand_model ?? '',
  panelPowerW: plant.panel_power_w ?? '',
  panelEfficiencyPct: plant.panel_efficiency_pct ?? '',
  panelCount: plant.panel_count == null ? '' : String(plant.panel_count),
  stringCount: plant.string_count == null ? '' : String(plant.string_count),
  orientation: plant.orientation ?? '',
  tiltAngleDeg: plant.tilt_angle_deg ?? '',
  totalCapacityKw: plant.total_capacity_kw ?? '',
  installationDate: plant.installation_date ?? null,
  address: plant.address ?? '',
  latitude: plant.latitude ?? '',
  longitude: plant.longitude ?? '',
  yearlyTargetKwh: plant.yearly_target_kwh ?? '',
  plantKind: plant.plant_kind,
  monthlyTargets: plant.monthly_targets.length === 12 ? plant.monthly_targets : Array.from({ length: 12 }, () => ''),
  alarmRecipients: plant.alarm_recipients,
  nettingAnalyzerId: plant.netting_analyzer_id ?? '',
  savedNettingAnalyzerId: plant.netting_analyzer_id ?? undefined,
});

/** Seeds the price fields from the plant's newest solar tariff. */
export const withTariff = (draft: PlantDraft, tariff?: SolarTariff): PlantDraft =>
  tariff
    ? { ...draft, priceMwh: shiftDecimal(tariff.feed_in_tariff, 3), currency: (tariff.currency as Currency) || 'TRY', savedTariff: tariff }
    : draft;

/** The solar tariff to store after a save, or null when the price is empty or unchanged. */
export function toTariffRequest(draft: PlantDraft, plantId: string, today: string): SolarTariffFields | null {
  if (draft.priceMwh === '') return null;
  const perKwh = shiftDecimal(draft.priceMwh, -3);
  const saved = draft.savedTariff;
  if (saved && !draft.priceFrom && shiftDecimal(saved.feed_in_tariff, 0) === perKwh && saved.currency === draft.currency) return null;
  const from = draft.priceFrom ?? (saved ? today : (draft.installationDate ?? today));
  return { plant_id: plantId, effective_from: from, feed_in_tariff: perKwh, currency: draft.currency };
}

/** Every month needs a target, so the request never carries a substituted zero. */
export const monthlyTargetsComplete = (draft: PlantDraft) =>
  draft.monthlyTargets.length === 12 && draft.monthlyTargets.every((value) => value !== '');

export function toPlantRequest(draft: PlantDraft): PlantCreateRequest {
  return {
    name: draft.name,
    installation_number: draft.installationNumber || null,
    pv_brand_model: draft.pvBrandModel || null,
    ...(draft.panelPowerW ? { panel_power_w: draft.panelPowerW } : {}),
    ...(draft.panelEfficiencyPct ? { panel_efficiency_pct: draft.panelEfficiencyPct } : {}),
    panel_count: draft.panelCount === '' ? null : Number(draft.panelCount),
    string_count: draft.stringCount === '' ? null : Number(draft.stringCount),
    ...(draft.orientation ? { orientation: draft.orientation } : {}),
    ...(draft.tiltAngleDeg ? { tilt_angle_deg: draft.tiltAngleDeg } : {}),
    ...(draft.totalCapacityKw ? { total_capacity_kw: draft.totalCapacityKw } : {}),
    ...(draft.installationDate ? { installation_date: draft.installationDate } : {}),
    address: draft.address || null,
    ...(draft.latitude ? { latitude: draft.latitude } : {}),
    ...(draft.longitude ? { longitude: draft.longitude } : {}),
    ...(draft.yearlyTargetKwh ? { yearly_target_kwh: draft.yearlyTargetKwh } : {}),
    plant_kind: draft.plantKind,
    monthly_targets: draft.monthlyTargets,
    alarm_recipients: draft.alarmRecipients,
    ...(draft.nettingAnalyzerId ? { netting_analyzer_id: draft.nettingAnalyzerId } : {}),
    ...(!draft.nettingAnalyzerId && draft.savedNettingAnalyzerId ? { clear_netting_analyzer: true } : {}),
  };
}

const NO_ANALYZER = 'none';

export type PlantFormViewProps = {
  value: PlantDraft;
  onChange: (draft: PlantDraft) => void;
  devices: PlantDevice[];
  fieldErrors?: Record<string, string>;
  /** The company's analyzers for the netting choice (R289). */
  analyzers?: { value: string; label: string }[];
};

/** Plant fields of 01 §7.15; devices arrive with the iSolar link (F9). */
export function PlantFormView({ value, onChange, devices, fieldErrors = {}, analyzers = [] }: PlantFormViewProps) {
  const t = useTranslations('settings.plants');
  const forms = useTranslations('forms');
  const locale = useLocale() as Locale;
  const [recipient, setRecipient] = useState('');
  const set = (patch: Partial<PlantDraft>) => onChange({ ...value, ...patch });
  const recipientInvalid = recipient !== '' && !EMAIL.test(recipient);

  return (
    <div className="flex flex-col gap-4">
      <Input label={t('name')} value={value.name} error={fieldErrors.name} onChange={(e) => set({ name: e.target.value })} required />
      <Input label={t('installationNumber')} error={fieldErrors.installation_number} value={value.installationNumber} onChange={(e) => set({ installationNumber: e.target.value })} />
      <Select
        label={t('kind')}
        error={fieldErrors.plant_kind}
        options={[
          { value: 'rooftop', label: t('rooftop') },
          { value: 'grid', label: t('grid') },
        ]}
        value={value.plantKind}
        onValueChange={(kind) => set({ plantKind: kind as PlantDraft['plantKind'] })}
      />
      <Select
        label={t('nettingAnalyzer')}
        description={t('nettingAnalyzerHint')}
        error={fieldErrors.netting_analyzer_id}
        options={[{ value: NO_ANALYZER, label: t('nettingNone') }, ...analyzers]}
        value={value.nettingAnalyzerId || NO_ANALYZER}
        onValueChange={(id) => set({ nettingAnalyzerId: id === NO_ANALYZER ? '' : id })}
      />
      <Input label={t('pvBrandModel')} error={fieldErrors.pv_brand_model} value={value.pvBrandModel} onChange={(e) => set({ pvBrandModel: e.target.value })} />
      <div className="flex flex-wrap gap-4">
        <NumberInput label={t('panelPower')} value={value.panelPowerW} error={fieldErrors.panel_power_w} onValueChange={(v) => set({ panelPowerW: v ?? '' })} fractionDigits={2} />
        <NumberInput label={t('panelEfficiency')} value={value.panelEfficiencyPct} error={fieldErrors.panel_efficiency_pct} onValueChange={(v) => set({ panelEfficiencyPct: v ?? '' })} fractionDigits={2} />
        <NumberInput label={t('panelCount')} value={value.panelCount} error={fieldErrors.panel_count} min="0" onValueChange={(v) => set({ panelCount: v ?? '' })} fractionDigits={0} />
        <NumberInput label={t('stringCount')} value={value.stringCount} error={fieldErrors.string_count} min="0" onValueChange={(v) => set({ stringCount: v ?? '' })} fractionDigits={0} />
      </div>
      <div className="flex flex-wrap gap-4">
        <Select
          label={t('orientation')}
          error={fieldErrors.orientation}
          options={ORIENTATIONS.map((value) => ({ value, label: t(`orientations.${value}`) }))}
          value={value.orientation || null}
          onValueChange={(orientation) => set({ orientation: orientation as PlantDraft['orientation'] })}
        />
        <NumberInput label={t('tilt')} value={value.tiltAngleDeg} error={fieldErrors.tilt_angle_deg} onValueChange={(v) => set({ tiltAngleDeg: v ?? '' })} fractionDigits={2} />
        <NumberInput label={t('capacity')} value={value.totalCapacityKw} error={fieldErrors.total_capacity_kw} onValueChange={(v) => set({ totalCapacityKw: v ?? '' })} fractionDigits={2} />
      </div>
      <DatePicker label={t('installationDate')} error={fieldErrors.installation_date} value={value.installationDate} onValueChange={(date) => set({ installationDate: date })} />
      <Textarea label={t('address')} error={fieldErrors.address} value={value.address} rows={2} onChange={(e) => set({ address: e.target.value })} />
      <div className="flex flex-wrap gap-4">
        <NumberInput label={t('latitude')} value={value.latitude} error={fieldErrors.latitude} onValueChange={(v) => set({ latitude: v ?? '' })} min="-90" max="90" fractionDigits={6} />
        <NumberInput label={t('longitude')} value={value.longitude} error={fieldErrors.longitude} onValueChange={(v) => set({ longitude: v ?? '' })} min="-180" max="180" fractionDigits={6} />
      </div>
      <NumberInput label={t('yearlyTarget')} value={value.yearlyTargetKwh} error={fieldErrors.yearly_target_kwh} onValueChange={(v) => set({ yearlyTargetKwh: v ?? '' })} fractionDigits={2} />

      <fieldset className="flex flex-col gap-3">
        <legend className="text-foreground type-small font-semibold">{t('monthlyTargets')} <span className="font-normal text-foreground-muted">({forms('required')})</span></legend>
        <div className="grid gap-3 sm:grid-cols-3">
          {MONTH_KEYS.map((month, index) => (
            <NumberInput
              key={month}
              label={t(`months.${month}`)}
              error={fieldErrors[`monthly_targets[${index}]`] ?? (index === 0 ? fieldErrors.monthly_targets : undefined)}
              required
              value={value.monthlyTargets[index] ?? ''}
              onValueChange={(v) => set({ monthlyTargets: value.monthlyTargets.map((target, i) => (i === index ? (v ?? '') : target)) })}
              fractionDigits={2}
            />
          ))}
        </div>
        {!monthlyTargetsComplete(value) ? <p className="text-danger type-small">{t('monthlyTargetsRequired')}</p> : null}
        {fieldErrors.monthly_targets ? <p className="text-danger type-small">{fieldErrors.monthly_targets}</p> : null}
      </fieldset>

      <fieldset className="flex flex-col gap-3">
        <legend className="text-foreground type-small font-semibold">{t('finance.title')}</legend>
        <p className="text-foreground-muted type-small">{t('finance.hint')}</p>
        <div className="flex flex-wrap gap-4">
          <NumberInput
            label={t('finance.price')}
            value={value.priceMwh}
            error={fieldErrors.feed_in_tariff}
            min="0"
            onValueChange={(v) => set({ priceMwh: v ?? '' })}
            fractionDigits={2}
          />
          <Select
            label={t('finance.currency')}
            error={fieldErrors.currency}
            options={CURRENCIES.map((c) => ({ value: c, label: c }))}
            value={value.currency}
            onValueChange={(currency) => set({ currency: currency as Currency })}
          />
          <DatePicker
            label={t('finance.from')}
            description={value.savedTariff ? t('finance.fromHintChange') : t('finance.fromHintNew')}
            value={value.priceFrom}
            error={fieldErrors.effective_from}
            onValueChange={(date) => set({ priceFrom: date })}
          />
        </div>
        {value.savedTariff ? (
          <p className="text-foreground-muted type-small">
            {t('finance.current', {
              price: shiftDecimal(value.savedTariff.feed_in_tariff, 3),
              currency: value.savedTariff.currency ?? 'TRY',
              date: formatDate(value.savedTariff.effective_from, locale),
            })}
          </p>
        ) : null}
      </fieldset>

      <fieldset className="flex flex-col gap-3">
        <legend className="text-foreground type-small font-semibold">{t('alarmRecipients')}</legend>
        <ul className="flex flex-wrap gap-2">
          {value.alarmRecipients.map((email) => (
            <li key={email} className="flex items-center gap-1 rounded-md border border-border px-2 py-1 type-small">
              {email}
              <IconButton
                label={t('removeRecipient', { email })}
                icon={Trash2}
                size="sm"
                onClick={() => set({ alarmRecipients: value.alarmRecipients.filter((r) => r !== email) })}
              />
            </li>
          ))}
        </ul>
        <div className="flex flex-wrap items-end gap-2">
          <Input
            label={t('addRecipient')}
            type="email"
            value={recipient}
            error={recipientInvalid ? t('invalidEmail') : undefined}
            onChange={(e) => setRecipient(e.target.value)}
          />
          <Button
            variant="secondary"
            size="sm"
            disabled={recipient === '' || recipientInvalid}
            onClick={() => {
              set({ alarmRecipients: [...value.alarmRecipients, recipient] });
              setRecipient('');
            }}
          >
            {t('addRecipient')}
          </Button>
        </div>
      </fieldset>

      {value.id ? (
        <section className="flex flex-col gap-2">
          <h3 className="text-foreground type-h3">{t('devices')}</h3>
          <p className="text-foreground-muted type-small">{t('devicesFromIsolar')}</p>
          {devices.length > 0 ? (
            <TableContainer label={t('devices')}>
              <Table>
                <TableCaption>{t('devices')}</TableCaption>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('deviceName')}</TableHead>
                    <TableHead>{t('brand')}</TableHead>
                    <TableHead>{t('model')}</TableHead>
                    <TableHead numeric>{t('power')}</TableHead>
                    <TableHead>{t('status')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {devices.map((device) => (
                    <TableRow key={device.id}>
                      <TableCell>{device.device_name ?? device.device_sn}</TableCell>
                      <TableCell>{device.brand ?? '—'}</TableCell>
                      <TableCell>{device.model ?? '—'}</TableCell>
                      <TableCell numeric>{device.rated_power_kw ?? '—'}</TableCell>
                      <TableCell>{device.status ?? '—'}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          ) : null}
        </section>
      ) : null}
    </div>
  );
}
