import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { ProgressBar } from './progress-bar';

describe('ProgressBar', () => {
  it('null value is indeterminate', () => {
    const { getByRole, getByText, rerender } = renderWithProviders(
      <ProgressBar label="Endüktif oran" value={null} thresholds={[{ value: 20, label: 'Sınır %20' }]} />,
    );
    const bar = getByRole('progressbar', { name: 'Endüktif oran' });
    expect(bar).not.toHaveAttribute('aria-valuenow');
    expect(getByText('Sınır %20')).toBeInTheDocument();
    rerender(<ProgressBar label="Endüktif oran" value={25} valueText="%25" />);
    expect(getByRole('progressbar')).toHaveAttribute('aria-valuenow', '25');
    expect(getByRole('progressbar')).toHaveAttribute('aria-valuetext', '%25');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<ProgressBar label="Yükleme" value={40} thresholds={[{ value: 80, label: 'Uyarı' }]} />);
    await expectNoAxeViolations(container);
  });
});
