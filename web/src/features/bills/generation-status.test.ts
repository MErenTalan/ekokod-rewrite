import { describe, expect, it } from 'vitest';

import { messageKeyForErrorCode, validateSelection } from './generation-status';

describe('§7.10 validation messages', () => {
  it('names every selection failure before anything is enqueued', () => {
    expect(validateSelection({ analyzerIds: [], buildingId: 'b', period: '2026-08', scope: 'analyzer' })).toBe('noAnalyzerSelected');
    expect(validateSelection({ analyzerIds: ['a'], buildingId: 'b', period: null, scope: 'analyzer' })).toBe('noMonthSelected');
    expect(validateSelection({ analyzerIds: [], buildingId: null, period: '2026-08', scope: 'building' })).toBe('noBuildingSelected');
    expect(validateSelection({ analyzerIds: ['a'], buildingId: 'b', period: '2026-8', scope: 'analyzer' })).toBe('invalidMonthFormat');
    expect(validateSelection({ analyzerIds: ['a'], buildingId: 'b', period: '2026-08', scope: 'analyzer' })).toBeNull();
  });

  it('does not ask a company-scope generation for a building or an analyzer', () => {
    expect(validateSelection({ analyzerIds: [], buildingId: null, period: '2026-08', scope: 'company' })).toBeNull();
  });

  it('maps every R113 code the API can answer with', () => {
    expect(messageKeyForErrorCode('tariff_not_found')).toBe('noTariffForBuilding');
    expect(messageKeyForErrorCode('no_consumption_data')).toBe('noConsumptionData');
    expect(messageKeyForErrorCode('unresolved_anomaly')).toBe('unresolvedAnomaly');
    expect(messageKeyForErrorCode('period_not_closed')).toBe('periodNotClosed');
    expect(messageKeyForErrorCode('ptf_data_missing')).toBe('ptfDataMissing');
    expect(messageKeyForErrorCode('billing_parameters_missing')).toBe('generationFailed');
  });

  it('still says something for a code it has never seen', () => {
    // An empty toast is worse than a generic sentence.
    expect(messageKeyForErrorCode('something_new')).toBe('generationFailed');
    expect(messageKeyForErrorCode(undefined)).toBe('generationFailed');
  });
});
