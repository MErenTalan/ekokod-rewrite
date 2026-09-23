'use client';

import { useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import type { ExtraChargeBasis } from '@/lib/api/types';

import { applyMode, draftErrors, type TariffDraft } from './tariff-draft';
import { optionKey } from './tariff-labels';

export type TariffFormViewProps = {
  draft: TariffDraft;
  onDraftChange: (draft: TariffDraft) => void;
  onSubmit: () => void;
  /** Field → message, straight from the API's 422 details (R199). */
  serverErrors?: Record<string, string>;
  readOnly?: boolean;
  saving?: boolean;
  onCancel?: () => void;
};

const CURRENCIES = ['TRY', 'USD', 'EUR'] as const;
const ENERGY_TYPES = ['grid_energy', 'green_energy'] as const;
const VOLTAGE_LEVELS = ['lv', 'mv'] as const;
const USER_GROUPS = [
  'residential', 'residential_plus', 'commercial', 'commercial_plus', 'industrial',
  'agricultural', 'lighting', 'martyrs_families', 'public_lighting',
] as const;
const PRICE_TYPES = ['single_time', 'multi_time'] as const;
const TERMS = ['monomial', 'binomial'] as const;
const SUPPLY_COMPANIES = ['incumbent', 'private'] as const;
const GENERATION_USAGES = ['none', 'subtract_from_consumption', 'subtract_from_total'] as const;
const PRICE_SOURCES = ['kbk', 'fixed'] as const;
const EXTRA_BASES: ExtraChargeBasis[] = ['per_kwh', 'per_contracted_kw', 'per_max_demand_kw', 'fixed_per_period', 'pct_of_energy'];

/**
 * The §7.11 tariff form: one form with mode switches rather than several
 * screens (R249). Switching a mode clears the abandoned mode's fields through
 * `applyMode`, so a fixed T1 price can never ride along on a PTF tariff.
 */
export function TariffFormView({
  draft, onDraftChange, onSubmit, serverErrors = {}, readOnly = false, saving = false, onCancel,
}: TariffFormViewProps) {
  const t = useTranslations('tariffs');
  const common = useTranslations('common');
  const forms = useTranslations('forms');
  const errors = draftErrors(draft);
  const codeMessages: Record<string, string> = {
    required: forms('errors.required'),
    must_be_empty: forms('errors.mustBeEmpty'),
    must_be_try: forms('errors.mustBeTry'),
    out_of_range: forms('errors.outOfRange'),
    duplicate: forms('errors.duplicate'),
    invalid: forms('errors.invalid'),
  };
  // The server's own message wins: it answered about the data that was sent.
  const errorText = (field: string) => serverErrors[field] ?? (errors[field] ? codeMessages[errors[field]] ?? codeMessages.invalid : undefined);

  const set = (patch: Partial<TariffDraft>) => onDraftChange({ ...draft, ...patch });
  const mode = (change: Parameters<typeof applyMode>[1]) => onDraftChange(applyMode(draft, change));

  const price = (field: keyof TariffDraft, labelKey: string, required = false) => (
    <Input
      label={t(labelKey as never)}
      required={required}
      inputMode="decimal"
      disabled={readOnly}
      value={String(draft[field] ?? '')}
      error={errorText(snake(field))}
      onChange={(e) => set({ [field]: e.target.value } as Partial<TariffDraft>)}
    />
  );

  const options = <T extends readonly string[]>(values: T, group: string) =>
    values.map((value) => ({ value, label: t(`options.${group}.${optionKey(value)}` as never) }));

  const ptf = draft.usePtfYekdem;

  return (
    <Card className="flex flex-col gap-6">
      <section className="flex flex-col gap-4">
        <h3 className="type-h3">{t('sections.identity')}</h3>
        <div className="grid gap-4 sm:grid-cols-2">
          <Input
            label={t('fields.effectiveFrom')}
            required
            type="date"
            disabled={readOnly}
            value={draft.effectiveFrom}
            error={errorText('effective_from')}
            onChange={(e) => set({ effectiveFrom: e.target.value })}
          />
          <Input
            label={t('fields.name')}
            disabled={readOnly}
            value={draft.name}
            error={errorText('name')}
            onChange={(e) => set({ name: e.target.value })}
          />
          <Select
            label={t('fields.currency')}
            required
            disabled={readOnly}
            value={draft.currency}
            description={ptf ? t('ptfCurrencyNote') : undefined}
            error={errorText('currency')}
            options={options(CURRENCIES, 'currency')}
            onValueChange={(value) => set({ currency: value as TariffDraft['currency'] })}
          />
        </div>
      </section>

      <section className="flex flex-col gap-4">
        <h3 className="type-h3">{t('sections.classification')}</h3>
        <div className="grid gap-4 sm:grid-cols-2">
          <Select label={t('fields.energyType')} required disabled={readOnly} value={draft.energyType}
            options={options(ENERGY_TYPES, 'energyType')}
            onValueChange={(value) => set({ energyType: value as TariffDraft['energyType'] })} />
          <Select label={t('fields.voltageLevel')} required disabled={readOnly} value={draft.voltageLevel}
            options={options(VOLTAGE_LEVELS, 'voltageLevel')}
            onValueChange={(value) => set({ voltageLevel: value as TariffDraft['voltageLevel'] })} />
          <Select label={t('fields.userGroup')} required disabled={readOnly} value={draft.userGroup}
            options={options(USER_GROUPS, 'userGroup')}
            onValueChange={(value) => set({ userGroup: value as TariffDraft['userGroup'] })} />
          <Select label={t('fields.supplyCompany')} required disabled={readOnly} value={draft.supplyCompany}
            options={options(SUPPLY_COMPANIES, 'supplyCompany')}
            onValueChange={(value) => set({ supplyCompany: value as TariffDraft['supplyCompany'] })} />
          <Select label={t('fields.priceType')} required disabled={readOnly} value={draft.priceType}
            options={options(PRICE_TYPES, 'priceType')}
            onValueChange={(value) => mode({ priceType: value as TariffDraft['priceType'] })} />
          <Select label={t('fields.term')} required disabled={readOnly} value={draft.term}
            options={options(TERMS, 'term')}
            onValueChange={(value) => mode({ term: value as TariffDraft['term'] })} />
        </div>
      </section>

      <section className="flex flex-col gap-4">
        <h3 className="type-h3">{t('sections.ptf')}</h3>
        <Switch
          label={t('fields.usePtfYekdem')}
          description={t('ptfDescription')}
          disabled={readOnly}
          checked={ptf}
          onCheckedChange={(checked) => mode({ usePtfYekdem: checked })}
        />
        {ptf ? (
          <div className="grid gap-4 sm:grid-cols-2">
            {price('kbkEnergy', 'fields.kbkEnergy', true)}
            {draft.priceType === 'multi_time' ? (
              <>
                {price('kbkT1', 'fields.kbkT1', true)}
                {price('kbkT2', 'fields.kbkT2', true)}
                {price('kbkT3', 'fields.kbkT3', true)}
              </>
            ) : null}
            <Select label={t('fields.distributionPriceSource')} disabled={readOnly} value={draft.distributionPriceSource}
              options={options(PRICE_SOURCES, 'priceSource')}
              onValueChange={(value) => set({ distributionPriceSource: value as 'kbk' | 'fixed' })} />
            {draft.distributionPriceSource === 'kbk' ? price('kbkDistributionCostTlPerKwh', 'fields.kbkDistributionCost', true) : null}
            <Select label={t('fields.reactivePriceSource')} disabled={readOnly} value={draft.reactivePriceSource}
              options={options(PRICE_SOURCES, 'priceSource')}
              onValueChange={(value) => set({ reactivePriceSource: value as 'kbk' | 'fixed' })} />
            {draft.reactivePriceSource === 'kbk' ? price('kbkReactivePower', 'fields.kbkReactivePower', true) : null}
            {price('kbkOverusePrice', 'fields.kbkOverusePrice')}
            <Switch
              label={t('fields.useManualYekdem')}
              disabled={readOnly}
              checked={draft.useManualYekdem}
              onCheckedChange={(checked) => set({ useManualYekdem: checked, manualYekdem: checked ? draft.manualYekdem : [] })}
            />
          </div>
        ) : null}
        {ptf && draft.useManualYekdem ? (
          <div className="flex flex-col gap-3">
            {draft.manualYekdem.map((row, i) => (
              <div key={i} className="grid items-end gap-3 sm:grid-cols-4">
                <Input label={t('fields.year')} inputMode="numeric" disabled={readOnly} value={row.year}
                  onChange={(e) => set({ manualYekdem: replace(draft.manualYekdem, i, { ...row, year: e.target.value }) })} />
                <Input label={t('fields.month')} inputMode="numeric" disabled={readOnly} value={row.month}
                  error={errorText(`manual_yekdem[${i}].month`) ?? errorText(`manual_yekdem[${i}]`)}
                  onChange={(e) => set({ manualYekdem: replace(draft.manualYekdem, i, { ...row, month: e.target.value }) })} />
                <Input label={t('fields.value')} inputMode="decimal" disabled={readOnly} value={row.value}
                  onChange={(e) => set({ manualYekdem: replace(draft.manualYekdem, i, { ...row, value: e.target.value }) })} />
                {readOnly ? null : (
                  <Button variant="ghost" onClick={() => set({ manualYekdem: remove(draft.manualYekdem, i) })}>{t('remove')}</Button>
                )}
              </div>
            ))}
            {readOnly ? null : (
              <Button variant="secondary" onClick={() => set({ manualYekdem: [...draft.manualYekdem, { year: '', month: '', value: '' }] })}>
                {t('addYekdem')}
              </Button>
            )}
          </div>
        ) : null}
      </section>

      <section className="flex flex-col gap-4">
        <h3 className="type-h3">{t('sections.prices')}</h3>
        <div className="grid gap-4 sm:grid-cols-2">
          {!ptf && draft.priceType === 'single_time' ? price('singleTimePrice', 'fields.singleTimePrice', true) : null}
          {!ptf && draft.priceType === 'multi_time' ? (
            <>
              {price('t1Price', 'fields.t1Price', true)}
              {price('t2Price', 'fields.t2Price', true)}
              {price('t3Price', 'fields.t3Price', true)}
            </>
          ) : null}
          {price('distributionCost', 'fields.distributionCost', true)}
          {price('reactivePowerPrice', 'fields.reactivePowerPrice', true)}
          {price('overuseThresholdKwhPerDay', 'fields.overuseThreshold')}
          {price('overusePrice', 'fields.overusePrice')}
          {draft.energyType === 'green_energy' ? price('greenEnergyPrice', 'fields.greenEnergyPrice') : null}
          {draft.energyType === 'green_energy' ? price('greenEnergyDistributionCost', 'fields.greenEnergyDistributionCost') : null}
          {price('vatRate', 'fields.vatRate', true)}
        </div>
      </section>

      {draft.term === 'binomial' ? (
        <section className="flex flex-col gap-4">
          <h3 className="type-h3">{t('sections.power')}</h3>
          <div className="grid gap-4 sm:grid-cols-2">
            {price('contractedPowerKw', 'fields.contractedPowerKw', true)}
            <Select label={t('fields.powerPriceSource')} disabled={readOnly} value={draft.powerPriceSource}
              options={options(PRICE_SOURCES, 'priceSource')}
              onValueChange={(value) => set({ powerPriceSource: value as 'kbk' | 'fixed' })} />
            {ptf && draft.powerPriceSource === 'kbk'
              ? price('kbkPowerPrice', 'fields.kbkPowerPrice', true)
              : price('powerUnitPrice', 'fields.powerUnitPrice', true)}
          </div>
        </section>
      ) : null}

      <section className="flex flex-col gap-4">
        <h3 className="type-h3">{t('sections.generation')}</h3>
        <div className="grid gap-4 sm:grid-cols-2">
          <Select label={t('fields.generationUsage')} required disabled={readOnly} value={draft.generationUsage}
            options={options(GENERATION_USAGES, 'generationUsage')}
            onValueChange={(value) => set({ generationUsage: value as TariffDraft['generationUsage'] })} />
          {draft.generationUsage === 'subtract_from_total' ? price('generationPricePerKwh', 'fields.generationPricePerKwh', true) : null}
        </div>
      </section>

      <section className="flex flex-col gap-4">
        <h3 className="type-h3">{t('sections.taxes')}</h3>
        {draft.taxes.map((tax, i) => (
          <div key={i} className="grid items-end gap-3 sm:grid-cols-3">
            <Input label={t('fields.taxName')} disabled={readOnly} value={tax.name} error={errorText(`taxes[${i}].name`)}
              onChange={(e) => set({ taxes: replace(draft.taxes, i, { ...tax, name: e.target.value }) })} />
            <Input label={t('fields.taxRate')} inputMode="decimal" disabled={readOnly} value={tax.rate} error={errorText(`taxes[${i}].rate`)}
              onChange={(e) => set({ taxes: replace(draft.taxes, i, { ...tax, rate: e.target.value }) })} />
            {readOnly ? null : <Button variant="ghost" onClick={() => set({ taxes: remove(draft.taxes, i) })}>{t('remove')}</Button>}
          </div>
        ))}
        {readOnly ? null : (
          <Button variant="secondary" onClick={() => set({ taxes: [...draft.taxes, { name: '', rate: '' }] })}>{t('addTax')}</Button>
        )}
      </section>

      <section className="flex flex-col gap-4">
        <h3 className="type-h3">{t('sections.extras')}</h3>
        {draft.extraCharges.map((charge, i) => (
          <div key={i} className="grid items-end gap-3 sm:grid-cols-4">
            <Input label={t('fields.extraName')} disabled={readOnly} value={charge.name} error={errorText(`extra_charges[${i}].name`)}
              onChange={(e) => set({ extraCharges: replace(draft.extraCharges, i, { ...charge, name: e.target.value }) })} />
            <Select label={t('fields.extraBasis')} disabled={readOnly} value={charge.basis} error={errorText(`extra_charges[${i}].basis`)}
              options={EXTRA_BASES.map((basis) => ({ value: basis as string, label: t(`options.extraBasis.${optionKey(basis as string)}` as never) }))}
              onValueChange={(value) => set({ extraCharges: replace(draft.extraCharges, i, { ...charge, basis: value as ExtraChargeBasis }) })} />
            <Input label={t('fields.extraAmount')} inputMode="decimal" disabled={readOnly} value={charge.amount}
              error={errorText(`extra_charges[${i}].amount`)}
              onChange={(e) => set({ extraCharges: replace(draft.extraCharges, i, { ...charge, amount: e.target.value }) })} />
            {readOnly ? null : <Button variant="ghost" onClick={() => set({ extraCharges: remove(draft.extraCharges, i) })}>{t('remove')}</Button>}
          </div>
        ))}
        {readOnly ? null : (
          <Button variant="secondary" onClick={() => set({ extraCharges: [...draft.extraCharges, { name: '', basis: 'per_kwh', amount: '' }] })}>
            {t('addExtra')}
          </Button>
        )}
      </section>

      {readOnly ? null : (
        <div className="flex justify-end gap-3">
          {onCancel ? <Button variant="ghost" onClick={onCancel}>{common('cancel')}</Button> : null}
          <Button loading={saving} disabled={Object.keys(errors).length > 0} onClick={onSubmit}>{common('save')}</Button>
        </div>
      )}
    </Card>
  );
}

function replace<T>(rows: T[], index: number, row: T): T[] {
  return rows.map((r, i) => (i === index ? row : r));
}

function remove<T>(rows: T[], index: number): T[] {
  return rows.filter((_, i) => i !== index);
}

/**
 * Draft field name → the API's field name, so one error map serves both.
 * Spelled out rather than derived: kbk_t1 and kbk_distribution_cost_tl_per_kwh
 * do not fall out of any camel-to-snake rule, and a field whose error silently
 * fails to match leaves the operator with no reason at all.
 */
const API_FIELD: Record<string, string> = {
  effectiveFrom: 'effective_from', name: 'name', currency: 'currency',
  singleTimePrice: 'single_time_price', t1Price: 't1_price', t2Price: 't2_price', t3Price: 't3_price',
  overusePrice: 'overuse_price', overuseThresholdKwhPerDay: 'overuse_threshold_kwh_per_day',
  distributionCost: 'distribution_cost', reactivePowerPrice: 'reactive_power_price',
  greenEnergyPrice: 'green_energy_price', greenEnergyDistributionCost: 'green_energy_distribution_cost',
  contractedPowerKw: 'contracted_power_kw', powerUnitPrice: 'power_unit_price',
  generationPricePerKwh: 'generation_price_per_kwh', vatRate: 'vat_rate',
  kbkEnergy: 'kbk_energy', kbkT1: 'kbk_t1', kbkT2: 'kbk_t2', kbkT3: 'kbk_t3',
  kbkPowerPrice: 'kbk_power_price', kbkOverusePrice: 'kbk_overuse_price',
  kbkReactivePower: 'kbk_reactive_power', kbkDistributionCostTlPerKwh: 'kbk_distribution_cost_tl_per_kwh',
};

function snake(field: string | number | symbol): string {
  return API_FIELD[String(field)] ?? String(field);
}
