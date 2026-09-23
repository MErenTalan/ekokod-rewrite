import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoDaily } from './_fixture';
import { HistoryTabView } from './history-tab';

const base = { granularity: 'day' as const, range: { from: '2026-09-01', to: '2026-09-17' }, series: demoDaily,
  onGranularityChange: () => {}, onRangeChange: () => {}, onExport: () => {} };

describe('HistoryTabView', () => {
  it('draws the series with its unit and table', () => {
    const r = renderWithProviders(<HistoryTabView {...base} />);
    expect(r.getByRole('heading', { name: /Geçmiş üretim/ })).toBeVisible();
  });

  it('offers hour, day and month and reports the choice', async () => {
    const onGranularityChange = vi.fn();
    const r = renderWithProviders(<HistoryTabView {...base} onGranularityChange={onGranularityChange} />);
    await userEvent.click(r.getByRole('radio', { name: 'Aylık' }));
    expect(onGranularityChange).toHaveBeenCalledWith('month');
  });

  it('refuses a range the API refuses (R284) before calling it', () => {
    const r = renderWithProviders(<HistoryTabView {...base} granularity="hour" range={{ from: '2026-08-01', to: '2026-09-17' }} />);
    expect(r.getByText(/Saatlik görünüm en fazla 31 gün/)).toBeVisible();
    expect(r.getByRole('button', { name: /Excel/ })).toBeDisabled();
  });

  it('downloads the series as Excel', async () => {
    const onExport = vi.fn();
    const r = renderWithProviders(<HistoryTabView {...base} onExport={onExport} />);
    await userEvent.click(r.getByRole('button', { name: /Excel/ }));
    expect(onExport).toHaveBeenCalled();
  });

  it('badges a series that mixes data sources', () => {
    const r = renderWithProviders(<HistoryTabView {...base} series={{ ...demoDaily, mixed_basis: true }} />);
    expect(r.getByText(/Tahmini|Eksik|Karışık/)).toBeVisible();
  });
});
