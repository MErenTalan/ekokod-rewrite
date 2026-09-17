import { describe, expect, it } from 'vitest';

import type { ConsumptionRow } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { AlarmCheckDialogView } from './alarm-check-dialog';

const row = { period_start: '2026-03-14T00:00:00+03:00', active_import: '1234.5' } as ConsumptionRow;

describe('AlarmCheckDialogView', () => {
  it('says the model is unavailable instead of inventing a verdict (R165)', () => {
    const r = renderWithProviders(
      <AlarmCheckDialogView
        open
        onOpenChange={() => {}}
        row={row}
        granularity="daily"
        result={{ available: false, reason: 'ml_service_unavailable' }}
      />,
    );
    expect(r.getByText('14 Mar 2026')).toBeInTheDocument();
    expect(r.getByText('1.234,5 kWh')).toBeInTheDocument();
    expect(r.getByText('Tahmin modeli şu anda kullanılamıyor.')).toBeInTheDocument();
  });

  it('shows nothing about a verdict while the check is running', () => {
    const r = renderWithProviders(
      <AlarmCheckDialogView open onOpenChange={() => {}} row={row} granularity="daily" result={null} loading />,
    );
    expect(r.queryByText('Tahmin modeli şu anda kullanılamıyor.')).toBeNull();
  });
});
