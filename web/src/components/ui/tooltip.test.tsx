import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Tooltip } from './tooltip';

describe('Tooltip', () => {
  it('describes its trigger on focus and closes on Escape', async () => {
    const { findByRole, getByRole, queryByRole, user } = renderWithProviders(
      <Tooltip content="Son 30 günün ortalaması">
        <button type="button">Ortalama</button>
      </Tooltip>,
    );
    await user.tab();
    expect(await findByRole('tooltip')).toHaveTextContent('Son 30 günün ortalaması');
    expect(getByRole('button')).toHaveAttribute('aria-describedby');
    await user.keyboard('{Escape}');
    expect(queryByRole('tooltip')).not.toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(
      <Tooltip content="Açıklama">
        <button type="button">Ortalama</button>
      </Tooltip>,
    );
    await expectNoAxeViolations(container);
  });
});
