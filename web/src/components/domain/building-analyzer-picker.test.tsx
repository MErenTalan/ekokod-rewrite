import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { BuildingAnalyzerPicker, type BuildingAnalyzerPickerProps } from './building-analyzer-picker';

const buildings = [
  { id: 'a', name: 'Merkez Bina', active: true },
  { id: 'b', name: 'Soğuk Hava Deposu', active: true },
  { id: 'c', name: 'Eski Depo', active: false },
];
const analyzers = [
  { id: 'a1', buildingId: 'a', name: 'Ana Pano', active: true },
  { id: 'a2', buildingId: 'a', name: 'Kompresör Hattı', active: false },
  { id: 'b1', buildingId: 'b', name: 'Soğutma Grubu', active: true },
];

function setup(props: Partial<BuildingAnalyzerPickerProps> = {}) {
  const onValueChange = vi.fn();
  const utils = renderWithProviders(
    <BuildingAnalyzerPicker
      buildings={buildings}
      analyzers={analyzers}
      value={{ buildingId: null, analyzerId: null }}
      onValueChange={onValueChange}
      activeOnly={false}
      onActiveOnlyChange={() => {}}
      {...props}
    />,
  );
  return { ...utils, onValueChange };
}

describe('BuildingAnalyzerPicker', () => {
  it('analyzers are filtered to the building', async () => {
    const { getByRole, findAllByRole, user } = setup({ value: { buildingId: 'a', analyzerId: null } });
    await user.click(getByRole('combobox', { name: 'Analizör' }));
    expect((await findAllByRole('option')).map((o) => o.textContent)).toEqual(['Ana Pano', 'Kompresör Hattı']);
  });

  it('the analyzer picker waits for a building', () => {
    const { getByRole, getByText } = setup();
    expect(getByRole('combobox', { name: 'Analizör' })).toBeDisabled();
    expect(getByText('Önce bir bina seçin')).toBeInTheDocument();
  });

  it('changing building clears a foreign analyzer', async () => {
    const { getByRole, findByRole, user, onValueChange } = setup({ value: { buildingId: 'a', analyzerId: 'a1' } });
    await user.click(getByRole('combobox', { name: 'Bina' }));
    await user.click(await findByRole('option', { name: 'Soğuk Hava Deposu' }));
    expect(onValueChange).toHaveBeenCalledWith({ buildingId: 'b', analyzerId: null });
  });

  it('active only hides passive entries', async () => {
    const { getByRole, findAllByRole, user } = setup({ value: { buildingId: 'a', analyzerId: null }, activeOnly: true });
    await user.click(getByRole('combobox', { name: 'Analizör' }));
    expect((await findAllByRole('option')).map((o) => o.textContent)).toEqual(['Ana Pano']);
  });

  it('has no axe violations', async () => {
    const { container } = setup({ value: { buildingId: 'a', analyzerId: 'a1' } });
    await expectNoAxeViolations(container);
  });
});
