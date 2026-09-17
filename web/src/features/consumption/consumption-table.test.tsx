import { describe, expect, it, vi } from 'vitest';

import { messages } from '../../../messages';
import type { ConsumptionRow } from '@/lib/api/types';
import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { CONSUMPTION_COLUMNS } from './columns';
import { ConsumptionTableView } from './consumption-table';

const row = (overrides: Partial<ConsumptionRow> = {}): ConsumptionRow =>
  ({
    period_start: '2026-03-14T00:00:00+03:00',
    period_end: '2026-03-15T00:00:00+03:00',
    active_import: '1234.5',
    reactive_inductive_import: '310.25',
    reactive_capacitive_import: '61.5',
    inductive_ratio: '0.2512',
    capacitive_ratio: '0.0498',
    max_demand_kw: '88.4',
    active_import_index: '105000',
    partial: false,
    source: 'load_profile',
    suspect_registers: [],
    ...overrides,
  }) as ConsumptionRow;

const view = (overrides: Partial<React.ComponentProps<typeof ConsumptionTableView>> = {}) => (
  <ConsumptionTableView
    rows={[row()]}
    granularity="daily"
    canCheckAlarm
    onCheckAlarm={() => {}}
    onExport={() => {}}
    onPrint={() => {}}
    {...overrides}
  />
);

describe('ConsumptionTableView', () => {
  it('renders every §7.3 column header', () => {
    const r = renderWithProviders(view());
    for (const column of CONSUMPTION_COLUMNS) {
      const label = messages.tr.consumption.columns[column.labelKey];
      expect(r.getAllByRole('columnheader', { name: new RegExp(label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')) }).length, label).toBeGreaterThan(0);
    }
  });

  it('formats ratios as percentages and values with Turkish separators', () => {
    const r = renderWithProviders(view());
    expect(r.getByText('%25,12')).toBeInTheDocument();
    expect(r.getByText('1.234,5')).toBeInTheDocument();
    expect(r.getByText('14 Mar 2026')).toBeInTheDocument();
  });

  it('marks a partial period and a suspect register', () => {
    const partial = renderWithProviders(view({ rows: [row({ partial: true })] }));
    expect(partial.getByText('Eksik veri')).toBeInTheDocument();
    partial.unmount();
    const suspect = renderWithProviders(view({ rows: [row({ suspect_registers: ['active_import'] })] }));
    expect(suspect.getByText('Şüpheli')).toBeInTheDocument();
  });

  it('offers the alarm check only to a role that may run it', async () => {
    const onCheckAlarm = vi.fn();
    const allowed = renderWithProviders(view({ onCheckAlarm }));
    await allowed.user.click(allowed.getByRole('button', { name: /İşlemler/ }));
    await allowed.user.click(await allowed.findByRole('menuitem', { name: 'Alarm kontrolü' }));
    expect(onCheckAlarm).toHaveBeenCalled();
    allowed.unmount();

    const denied = renderWithProviders(view({ canCheckAlarm: false }));
    expect(denied.queryByRole('button', { name: /İşlemler/ })).toBeNull();
  });

  it('exports and prints', async () => {
    const onExport = vi.fn();
    const onPrint = vi.fn();
    const r = renderWithProviders(view({ onExport, onPrint }));
    await r.user.click(r.getByRole('button', { name: 'Dışa aktar' }));
    await r.user.click(await r.findByRole('menuitem', { name: 'Excel' }));
    expect(onExport).toHaveBeenCalledWith('excel');
    await r.user.click(r.getByRole('button', { name: 'Yazdır' }));
    expect(onPrint).toHaveBeenCalled();
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(view());
    await expectNoAxeViolations(r.container);
  });
});
