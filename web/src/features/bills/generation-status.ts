/** The message keys the bills screen can show for a failed generation. */
export type BillsMessageKey =
  | 'noAnalyzerSelected'
  | 'noBuildingSelected'
  | 'noMonthSelected'
  | 'invalidMonthFormat'
  | 'noTariffForBuilding'
  | 'noConsumptionData'
  | 'unresolvedAnomaly'
  | 'periodNotClosed'
  | 'ptfDataMissing'
  | 'generationFailed';

export type GenerationScope = 'analyzer' | 'building' | 'company';

/**
 * R113's codes → §7.10's sentences. A code outside the map still says
 * something: an empty toast tells the operator nothing at all.
 */
const CODE_MESSAGES: Record<string, BillsMessageKey> = {
  tariff_not_found: 'noTariffForBuilding',
  no_consumption_data: 'noConsumptionData',
  unresolved_anomaly: 'unresolvedAnomaly',
  period_not_closed: 'periodNotClosed',
  ptf_data_missing: 'ptfDataMissing',
};

export function messageKeyForErrorCode(code: string | undefined): BillsMessageKey {
  return (code && CODE_MESSAGES[code]) || 'generationFailed';
}

const MONTH = /^\d{4}-(0[1-9]|1[0-2])$/;

/** The selection guards of §7.10, in the order the spec lists them. */
export function validateSelection(input: {
  analyzerIds: string[];
  buildingId: string | null;
  period: string | null;
  scope: GenerationScope;
}): BillsMessageKey | null {
  if (input.scope === 'analyzer' && input.analyzerIds.length === 0) return 'noAnalyzerSelected';
  if (input.scope === 'building' && !input.buildingId) return 'noBuildingSelected';
  if (!input.period) return 'noMonthSelected';
  if (!MONTH.test(input.period)) return 'invalidMonthFormat';
  return null;
}
