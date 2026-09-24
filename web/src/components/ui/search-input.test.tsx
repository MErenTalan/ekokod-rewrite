import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { SearchInput } from './search-input';

describe('SearchInput', () => {
  it('hidden label still names the field', () => {
    const { getByRole } = renderWithProviders(<SearchInput label="Ara" value="" onValueChange={() => {}} labelVisibility="hidden" />);
    expect(getByRole('searchbox', { name: 'Ara' })).toBeInTheDocument();
  });

  it('clear button empties the value', async () => {
    const onValueChange = vi.fn();
    const { getByRole, user } = renderWithProviders(<SearchInput label="Ara" value="bina" onValueChange={onValueChange} />);
    await user.click(getByRole('button', { name: 'Temizle' }));
    expect(onValueChange).toHaveBeenCalledWith('');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<SearchInput label="Ara" value="bina" onValueChange={() => {}} />);
    await expectNoAxeViolations(container);
  });
});
