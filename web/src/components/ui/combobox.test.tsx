import { within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Combobox, normaliseTurkish } from './combobox';

const options = [
  { value: 'ist', label: 'İstanbul Ofis' },
  { value: 'ank', label: 'Ankara Fabrika' },
  { value: 'izm', label: 'Izmir Depo', disabled: true },
];

function setup(value: string | null = null) {
  const onValueChange = vi.fn();
  const utils = renderWithProviders(
    <Combobox label="Bina" options={options} value={value} onValueChange={onValueChange} searchPlaceholder="Bina ara" emptyText="Bina bulunamadı" />,
  );
  return { ...utils, onValueChange };
}

describe('Combobox', () => {
  it.each(['istanbul', 'ISTANBUL', 'ıstanbul'])('filters by Turkish text case-insensitively, including dotless ı (%s)', async (query) => {
    const { getByRole, findByRole, user } = setup();
    await user.click(getByRole('combobox', { name: 'Bina' }));
    await user.type(await findByRole('combobox', { name: 'Bina ara' }), query);
    const list = getByRole('listbox');
    expect(within(list).getAllByRole('option').map((o) => o.textContent)).toEqual(['İstanbul Ofis']);
  });

  it('empty text is shown when nothing matches', async () => {
    const { getByRole, findByRole, getByText, user } = setup();
    await user.click(getByRole('combobox', { name: 'Bina' }));
    await user.type(await findByRole('combobox', { name: 'Bina ara' }), 'zzz');
    expect(getByText('Bina bulunamadı')).toBeInTheDocument();
  });

  it('selects with the keyboard and re-selecting clears', async () => {
    const { getByRole, findByRole, user, onValueChange } = setup();
    await user.click(getByRole('combobox', { name: 'Bina' }));
    await findByRole('listbox');
    await user.keyboard('{ArrowDown}{Enter}');
    expect(onValueChange).toHaveBeenLastCalledWith('ank');
    const second = setup('ist');
    await second.user.click(second.getAllByRole('combobox', { name: 'Bina' })[1]);
    await second.user.click(await second.findByRole('option', { name: 'İstanbul Ofis' }));
    expect(second.onValueChange).toHaveBeenLastCalledWith(null);
  });

  it('normalises Turkish case', () => {
    expect(normaliseTurkish('İSTANBUL')).toBe(normaliseTurkish('ıstanbul'));
  });

  it('has no axe violations when open', async () => {
    const { baseElement, getByRole, findByRole, user } = setup('ank');
    await user.click(getByRole('combobox', { name: 'Bina' }));
    await findByRole('listbox');
    await expectNoAxeViolations(baseElement);
  });
});
