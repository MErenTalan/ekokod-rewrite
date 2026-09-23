import type { Tariff, TariffFields } from '@/lib/api/types';

/** The §7.11 form's own state. Every price is a string end to end (R248): no
 *  price passes through a float on its way to an invoice. */
export type TariffDraft = {
  id?: string;
  buildingId: string;
  name: string;
  effectiveFrom: string;
  currency: NonNullable<TariffFields['currency']>;
  energyType: NonNullable<TariffFields['energy_type']>;
  voltageLevel: NonNullable<TariffFields['voltage_level']>;
  userGroup: NonNullable<TariffFields['user_group']>;
  priceType: NonNullable<TariffFields['price_type']>;
  term: NonNullable<TariffFields['term']>;
  supplyCompany: NonNullable<TariffFields['supply_company']>;
  generationUsage: NonNullable<TariffFields['generation_usage']>;

  singleTimePrice: string;
  t1Price: string;
  t2Price: string;
  t3Price: string;
  overusePrice: string;
  overuseThresholdKwhPerDay: string;
  distributionCost: string;
  reactivePowerPrice: string;
  greenEnergyPrice: string;
  greenEnergyDistributionCost: string;
  contractedPowerKw: string;
  powerUnitPrice: string;
  generationPricePerKwh: string;
  vatRate: string;

  usePtfYekdem: boolean;
  kbkEnergy: string;
  kbkT1: string;
  kbkT2: string;
  kbkT3: string;
  kbkPowerPrice: string;
  kbkOverusePrice: string;
  kbkReactivePower: string;
  kbkDistributionCostTlPerKwh: string;
  useManualYekdem: boolean;
  manualYekdem: { year: string; month: string; value: string }[];

  powerPriceSource: 'kbk' | 'fixed';
  reactivePriceSource: 'kbk' | 'fixed';
  distributionPriceSource: 'kbk' | 'fixed';

  taxes: { name: string; rate: string }[];
  extraCharges: { name: string; basis: NonNullable<TariffFields['extra_charges']>[number]['basis']; amount: string }[];
};

/** The KBK fields the PTF+YEKDEM mode owns; switching the mode off clears them. */
const KBK_FIELDS = [
  'kbkEnergy', 'kbkT1', 'kbkT2', 'kbkT3', 'kbkPowerPrice', 'kbkOverusePrice',
  'kbkReactivePower', 'kbkDistributionCostTlPerKwh',
] as const satisfies readonly (keyof TariffDraft)[];

export function emptyDraft(): TariffDraft {
  return {
    buildingId: '', name: '', effectiveFrom: '', currency: 'TRY', energyType: 'grid_energy',
    voltageLevel: 'lv', userGroup: 'commercial', priceType: 'single_time', term: 'monomial',
    supplyCompany: 'incumbent', generationUsage: 'none',
    singleTimePrice: '', t1Price: '', t2Price: '', t3Price: '', overusePrice: '',
    overuseThresholdKwhPerDay: '', distributionCost: '', reactivePowerPrice: '',
    greenEnergyPrice: '', greenEnergyDistributionCost: '', contractedPowerKw: '',
    powerUnitPrice: '', generationPricePerKwh: '', vatRate: '',
    usePtfYekdem: false, kbkEnergy: '', kbkT1: '', kbkT2: '', kbkT3: '', kbkPowerPrice: '',
    kbkOverusePrice: '', kbkReactivePower: '', kbkDistributionCostTlPerKwh: '',
    useManualYekdem: false, manualYekdem: [],
    powerPriceSource: 'kbk', reactivePriceSource: 'kbk', distributionPriceSource: 'kbk',
    taxes: [], extraCharges: [],
  };
}

const str = (v: unknown): string => (v === undefined || v === null ? '' : String(v));

/** Builds a draft from a stored version, for the edit form. */
export function draftFrom(t: Tariff): TariffDraft {
  const base = emptyDraft();
  return {
    ...base,
    id: t.id,
    buildingId: str(t.building_id),
    name: str(t.name),
    effectiveFrom: str(t.effective_from),
    currency: t.currency ?? base.currency,
    energyType: t.energy_type ?? base.energyType,
    voltageLevel: t.voltage_level ?? base.voltageLevel,
    userGroup: t.user_group ?? base.userGroup,
    priceType: t.price_type ?? base.priceType,
    term: t.term ?? base.term,
    supplyCompany: t.supply_company ?? base.supplyCompany,
    generationUsage: t.generation_usage ?? base.generationUsage,
    singleTimePrice: str(t.single_time_price),
    t1Price: str(t.t1_price),
    t2Price: str(t.t2_price),
    t3Price: str(t.t3_price),
    overusePrice: str(t.overuse_price),
    overuseThresholdKwhPerDay: str(t.overuse_threshold_kwh_per_day),
    distributionCost: str(t.distribution_cost),
    reactivePowerPrice: str(t.reactive_power_price),
    greenEnergyPrice: str(t.green_energy_price),
    greenEnergyDistributionCost: str(t.green_energy_distribution_cost),
    contractedPowerKw: str(t.contracted_power_kw),
    powerUnitPrice: str(t.power_unit_price),
    generationPricePerKwh: str(t.generation_price_per_kwh),
    vatRate: str(t.vat_rate),
    usePtfYekdem: t.use_ptf_yekdem ?? false,
    kbkEnergy: str(t.kbk_energy),
    kbkT1: str(t.kbk_t1),
    kbkT2: str(t.kbk_t2),
    kbkT3: str(t.kbk_t3),
    kbkPowerPrice: str(t.kbk_power_price),
    kbkOverusePrice: str(t.kbk_overuse_price),
    kbkReactivePower: str(t.kbk_reactive_power),
    kbkDistributionCostTlPerKwh: str(t.kbk_distribution_cost_tl_per_kwh),
    useManualYekdem: t.use_manual_yekdem ?? false,
    manualYekdem: (t.manual_yekdem ?? []).map((y) => ({ year: String(y.year), month: String(y.month), value: str(y.value) })),
    powerPriceSource: t.power_price_source ?? base.powerPriceSource,
    reactivePriceSource: t.reactive_price_source ?? base.reactivePriceSource,
    distributionPriceSource: t.distribution_price_source ?? base.distributionPriceSource,
    taxes: (t.taxes ?? []).map((x) => ({ name: x.name, rate: str(x.rate) })),
    extraCharges: (t.extra_charges ?? []).map((x) => ({ name: x.name, basis: x.basis, amount: str(x.amount) })),
  };
}

type ModeChange = Partial<Pick<TariffDraft, 'usePtfYekdem' | 'priceType' | 'term'>>;

/**
 * Switches a mode and clears the fields the abandoned mode owned (R249).
 *
 * Without this a fixed T1 price typed before the PTF switch would ride along
 * in the request, and a monomial tariff would carry the contracted power the
 * server answers `must_be_empty` for.
 */
export function applyMode(draft: TariffDraft, change: ModeChange): TariffDraft {
  let next: TariffDraft = { ...draft, ...change };
  if (change.usePtfYekdem === true) {
    // PTF replaces the fixed ENERGY prices only: the power and reactive sides
    // stay fixed unless their own source says otherwise (R119, R132).
    next = { ...next, singleTimePrice: '', t1Price: '', t2Price: '', t3Price: '' };
  }
  if (change.usePtfYekdem === false) {
    for (const field of KBK_FIELDS) next = { ...next, [field]: '' };
    next = { ...next, useManualYekdem: false, manualYekdem: [] };
  }
  if (change.priceType === 'single_time') {
    next = { ...next, t1Price: '', t2Price: '', t3Price: '', kbkT1: '', kbkT2: '', kbkT3: '' };
  }
  if (change.priceType === 'multi_time') {
    next = { ...next, singleTimePrice: '' };
  }
  if (change.term === 'monomial') {
    next = { ...next, contractedPowerKw: '', powerUnitPrice: '', kbkPowerPrice: '' };
  }
  return next;
}

const filled = (v: string) => v.trim().length > 0;

/** The 422 the server would answer, checked here first: field → code, using
 *  the API's own field names and codes so one message table serves both. */
export function draftErrors(draft: TariffDraft): Record<string, string> {
  const errors: Record<string, string> = {};
  const require_ = (field: string, value: string) => {
    if (!filled(value)) errors[field] = 'required';
  };
  require_('effective_from', draft.effectiveFrom);
  require_('distribution_cost', draft.distributionCost);
  require_('reactive_power_price', draft.reactivePowerPrice);
  require_('vat_rate', draft.vatRate);

  const ptf = draft.usePtfYekdem;
  if (draft.priceType === 'single_time') {
    if (!ptf) require_('single_time_price', draft.singleTimePrice);
  } else if (ptf) {
    require_('kbk_t1', draft.kbkT1);
    require_('kbk_t2', draft.kbkT2);
    require_('kbk_t3', draft.kbkT3);
  } else {
    require_('t1_price', draft.t1Price);
    require_('t2_price', draft.t2Price);
    require_('t3_price', draft.t3Price);
  }
  if (ptf) {
    require_('kbk_energy', draft.kbkEnergy);
    if (draft.currency !== 'TRY') errors.currency = 'must_be_try'; // R127: no FX in this system
    if (draft.distributionPriceSource === 'kbk') require_('kbk_distribution_cost_tl_per_kwh', draft.kbkDistributionCostTlPerKwh);
    if (draft.reactivePriceSource === 'kbk') require_('kbk_reactive_power', draft.kbkReactivePower);
  }
  if (draft.generationUsage === 'subtract_from_total') require_('generation_price_per_kwh', draft.generationPricePerKwh);

  if (draft.term === 'binomial') {
    require_('contracted_power_kw', draft.contractedPowerKw);
    if (ptf && draft.powerPriceSource === 'kbk') require_('kbk_power_price', draft.kbkPowerPrice);
    else require_('power_unit_price', draft.powerUnitPrice);
  } else {
    if (filled(draft.contractedPowerKw)) errors.contracted_power_kw = 'must_be_empty';
    if (filled(draft.powerUnitPrice)) errors.power_unit_price = 'must_be_empty';
  }
  if (filled(draft.overuseThresholdKwhPerDay)) require_('overuse_price', draft.overusePrice);

  draft.taxes.forEach((tax, i) => {
    if (filled(tax.name) || filled(tax.rate)) {
      if (!filled(tax.name)) errors[`taxes[${i}].name`] = 'required';
      if (!filled(tax.rate)) errors[`taxes[${i}].rate`] = 'required';
    }
  });
  draft.extraCharges.forEach((charge, i) => {
    if (filled(charge.name) || filled(charge.amount)) {
      if (!filled(charge.name)) errors[`extra_charges[${i}].name`] = 'required';
      if (!filled(charge.amount)) errors[`extra_charges[${i}].amount`] = 'required';
      // M-5: a per-contracted-kW charge needs a contracted power to apply to.
      if (charge.basis === 'per_contracted_kw' && !filled(draft.contractedPowerKw)) errors[`extra_charges[${i}].basis`] = 'required';
    }
  });
  const seen = new Set<string>();
  draft.manualYekdem.forEach((row, i) => {
    const month = Number(row.month);
    if (!Number.isInteger(month) || month < 1 || month > 12) errors[`manual_yekdem[${i}].month`] = 'out_of_range';
    const key = `${row.year}-${row.month}`;
    if (seen.has(key)) errors[`manual_yekdem[${i}]`] = 'duplicate';
    seen.add(key);
  });
  return errors;
}

/** Maps the draft onto the request body; a blank optional is omitted, never
 *  sent as "0" — a zero price is a price the operator did not type. */
export function toTariffRequest(draft: TariffDraft): TariffFields {
  const body: Record<string, unknown> = {
    effective_from: draft.effectiveFrom,
    currency: draft.currency,
    energy_type: draft.energyType,
    voltage_level: draft.voltageLevel,
    user_group: draft.userGroup,
    price_type: draft.priceType,
    term: draft.term,
    supply_company: draft.supplyCompany,
    generation_usage: draft.generationUsage,
    use_ptf_yekdem: draft.usePtfYekdem,
    use_manual_yekdem: draft.useManualYekdem,
    power_price_source: draft.powerPriceSource,
    reactive_price_source: draft.reactivePriceSource,
    distribution_price_source: draft.distributionPriceSource,
    taxes: draft.taxes.filter((t) => filled(t.name) || filled(t.rate)).map((t) => ({ name: t.name, rate: t.rate })),
    extra_charges: draft.extraCharges
      .filter((c) => filled(c.name) || filled(c.amount))
      .map((c) => ({ name: c.name, basis: c.basis, amount: c.amount })),
    manual_yekdem: draft.usePtfYekdem
      ? draft.manualYekdem
          .filter((y) => filled(y.value))
          .map((y) => ({ year: Number(y.year), month: Number(y.month), value: y.value }))
      : [],
  };
  if (filled(draft.buildingId)) body.building_id = draft.buildingId;
  if (filled(draft.name)) body.name = draft.name;

  const optional: [string, string][] = [
    ['single_time_price', draft.singleTimePrice], ['t1_price', draft.t1Price], ['t2_price', draft.t2Price],
    ['t3_price', draft.t3Price], ['overuse_price', draft.overusePrice],
    ['overuse_threshold_kwh_per_day', draft.overuseThresholdKwhPerDay], ['distribution_cost', draft.distributionCost],
    ['reactive_power_price', draft.reactivePowerPrice], ['green_energy_price', draft.greenEnergyPrice],
    ['green_energy_distribution_cost', draft.greenEnergyDistributionCost], ['contracted_power_kw', draft.contractedPowerKw],
    ['power_unit_price', draft.powerUnitPrice], ['generation_price_per_kwh', draft.generationPricePerKwh],
    ['vat_rate', draft.vatRate], ['kbk_energy', draft.kbkEnergy], ['kbk_t1', draft.kbkT1], ['kbk_t2', draft.kbkT2],
    ['kbk_t3', draft.kbkT3], ['kbk_power_price', draft.kbkPowerPrice], ['kbk_overuse_price', draft.kbkOverusePrice],
    ['kbk_reactive_power', draft.kbkReactivePower], ['kbk_distribution_cost_tl_per_kwh', draft.kbkDistributionCostTlPerKwh],
  ];
  for (const [key, value] of optional) {
    if (filled(value)) body[key] = value;
  }
  return body as TariffFields;
}
