import { Bell } from 'lucide-react';
import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { IconButton } from './icon-button';

describe('IconButton', () => {
  it('label is the accessible name', () => {
    const { getByRole } = renderWithProviders(<IconButton label="Bildirimler" icon={Bell} />);
    expect(getByRole('button', { name: 'Bildirimler' })).toBeInTheDocument();
  });

  it('tooltip shows the label on keyboard focus', async () => {
    const { findByRole, user } = renderWithProviders(<IconButton label="Bildirimler" icon={Bell} />);
    await user.tab();
    expect(await findByRole('tooltip')).toHaveTextContent('Bildirimler');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<IconButton label="Bildirimler" icon={Bell} />);
    await expectNoAxeViolations(container);
  });
});
