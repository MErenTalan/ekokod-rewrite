import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { AnnouncementBar } from './announcement-bar';

describe('AnnouncementBar', () => {
  it('announces the calculator and links to it', async () => {
    const { getByRole, container } = renderWithProviders(<AnnouncementBar />);
    expect(getByRole('region', { name: /Yeni: elektrik faturanızı/ })).toBeInTheDocument();
    expect(getByRole('link', { name: 'Fatura hesaplayıcıyı dene' })).toHaveAttribute('href', '/bill-calculator');
    await expectNoAxeViolations(container);
  });
});
