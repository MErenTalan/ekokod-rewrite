import { screen } from '@testing-library/react';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { ReportSelectionView, type ReportSelection } from './report-selection';

const buildings = [
  { value: 'b-1', label: 'Merkez' },
  { value: 'b-2', label: 'Depo' },
];
const plants = [
  { value: 'p-1', label: 'Arazi GES', kind: 'grid' as const },
  { value: 'p-2', label: 'Çatı GES', kind: 'rooftop' as const },
];
const start: ReportSelection = { buildingIds: ['b-1'], period: '2026-08', plantSelection: 'all', plantIds: [] };

function Harness({ onChange = () => {}, withPlants = true }: { onChange?: (v: ReportSelection) => void; withPlants?: boolean }) {
  const [value, setValue] = useState(start);
  return (
    <ReportSelectionView
      kind="monthly"
      buildings={buildings}
      plants={withPlants ? plants : null}
      value={value}
      onChange={(v) => {
        setValue(v);
        onChange(v);
      }}
    />
  );
}

describe('ReportSelectionView', () => {
  it('selects every building at once', async () => {
    const onChange = vi.fn();
    const { user } = renderWithProviders(<Harness onChange={onChange} />);
    await user.click(screen.getByRole('button', { name: 'Tümünü seç' }));
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ buildingIds: ['b-1', 'b-2'] }));
  });

  it('never leaves the report without a building', async () => {
    const onChange = vi.fn();
    const { user } = renderWithProviders(<Harness onChange={onChange} />);
    await user.click(screen.getByRole('combobox', { name: 'Binalar' }));
    await user.click(await screen.findByRole('option', { name: 'Merkez' })); // unselect the only one
    expect(onChange).not.toHaveBeenCalledWith(expect.objectContaining({ buildingIds: [] }));
  });

  it('counts the selected plants and says rooftop production is included', async () => {
    const { user } = renderWithProviders(<Harness />);
    expect(screen.getByText('Santral seçilmedi: tüm santraller dahil')).toBeVisible();
    expect(screen.getByText('Çatı GES üretimi dahil edilecek.')).toBeVisible();
    await user.click(screen.getByRole('combobox', { name: 'Santraller' }));
    await user.click(await screen.findByRole('option', { name: 'Arazi GES' }));
    expect(screen.getByText('1 santral seçili')).toBeVisible();
  });

  it('lists only utility-scale plants and drops the rooftop note for the grid selection', async () => {
    const { user } = renderWithProviders(<Harness />);
    await user.click(screen.getByRole('radio', { name: 'Yalnızca arazi GES' }));
    expect(screen.queryByText('Çatı GES üretimi dahil edilecek.')).toBeNull();
    await user.click(screen.getByRole('combobox', { name: 'Santraller' }));
    expect(await screen.findByRole('option', { name: 'Arazi GES' })).toBeVisible();
    expect(screen.queryByRole('option', { name: 'Çatı GES' })).toBeNull();
  });

  it('offers no plant picker to a principal who may not list plants', () => {
    renderWithProviders(<Harness withPlants={false} />);
    expect(screen.queryByRole('combobox', { name: 'Santraller' })).toBeNull();
    expect(screen.getByRole('group', { name: 'Santral seçimi' })).toBeVisible();
  });
});
