import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { FilterBar } from './filter-bar';

describe('FilterBar', () => {
  it('small screens get a filter sheet with the active count', async () => {
    const onApply = vi.fn();
    const { getByRole, findByRole, user } = renderWithProviders(
      <FilterBar activeCount={2} onApply={onApply}>
        <label>
          Bina <input />
        </label>
      </FilterBar>,
    );
    await user.click(getByRole('button', { name: 'Filtreler (2)' }));
    const sheet = await findByRole('dialog', { name: 'Filtreler' });
    await user.click(sheet.querySelector('[data-apply]')!);
    expect(onApply).toHaveBeenCalled();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(
      <FilterBar onApply={() => {}}>
        <span>Filtre</span>
      </FilterBar>,
    );
    await expectNoAxeViolations(container);
  });
});
