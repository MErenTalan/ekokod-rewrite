import { describe, expect, it } from 'vitest';

import type { ConsumptionSummary } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { SummaryCardsView } from './summary-cards';

const summary = (suspect = 0): ConsumptionSummary =>
  ({
    rows: 31,
    suspect_rows: suspect,
    totals: { active_import: '18450.5', reactive_inductive_import: '3321.09', reactive_capacitive_import: '553.5' },
    averages: { active_import: '595.18' },
  }) as ConsumptionSummary;

describe('SummaryCardsView', () => {
  it('shows the four §7.3 figures with Turkish separators', () => {
    const r = renderWithProviders(<SummaryCardsView summary={summary()} />);
    expect(r.getByText('18.450,5')).toBeInTheDocument();
    expect(r.getByText('3.321,09')).toBeInTheDocument();
    expect(r.getByText('553,5')).toBeInTheDocument();
    expect(r.getByText('595,18')).toBeInTheDocument();
    expect(r.queryByText('Şüpheli')).toBeNull();
  });

  it('marks every figure when any period is suspect', () => {
    const r = renderWithProviders(<SummaryCardsView summary={summary(2)} />);
    expect(r.getAllByText('Şüpheli')).toHaveLength(4);
  });
});
