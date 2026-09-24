import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { StatusBadge } from './status-badge';

describe('StatusBadge', () => {
  it('status is never colour alone', () => {
    const { getByText } = renderWithProviders(<StatusBadge status="danger" label="Sınır aşıldı" />);
    const badge = getByText('Sınır aşıldı').closest('span[class]')!;
    expect(badge.querySelector('svg[aria-hidden="true"]')).not.toBeNull();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<StatusBadge status="success" label="Sınır içinde" />);
    await expectNoAxeViolations(container);
  });
});
