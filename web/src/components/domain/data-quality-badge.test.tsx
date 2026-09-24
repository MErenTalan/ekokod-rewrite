import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { DataQualityBadge } from './data-quality-badge';

describe('DataQualityBadge', () => {
  it('complete renders nothing', () => {
    const { container } = renderWithProviders(<DataQualityBadge quality={{ state: 'complete' }} />);
    expect(container.querySelector('[data-quality-badge]')).toBeNull();
  });

  it('reason is keyboard reachable', async () => {
    const { findByRole, getByText, user } = renderWithProviders(
      <DataQualityBadge quality={{ state: 'estimated', reason: '3 saatlik ölçüm eksik, tahminle dolduruldu', coverage: '97.5' }} />,
    );
    expect(getByText('Tahmini')).toBeInTheDocument();
    await user.tab();
    const tooltip = await findByRole('tooltip');
    expect(tooltip).toHaveTextContent('3 saatlik ölçüm eksik, tahminle dolduruldu');
    expect(tooltip).toHaveTextContent('Veri kapsamı %97,5');
  });

  it('suspect is danger toned', () => {
    const { getByText } = renderWithProviders(<DataQualityBadge quality={{ state: 'suspect', reason: 'Sayaç sıfırlandı' }} />);
    expect(getByText('Şüpheli').closest('[data-quality-badge]')).toHaveClass('text-danger');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<DataQualityBadge quality={{ state: 'incomplete', reason: 'Eksik gün' }} />);
    await expectNoAxeViolations(container);
  });
});
