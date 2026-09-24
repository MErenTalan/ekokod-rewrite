import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Stepper } from './stepper';

const steps = [
  { id: 'file', label: 'Dosya' },
  { id: 'map', label: 'Eşleştirme', description: 'Sütunları eşleyin' },
  { id: 'review', label: 'Önizleme' },
];

describe('Stepper', () => {
  it('marks the current step', () => {
    const { getByText, getAllByRole } = renderWithProviders(<Stepper steps={steps} currentIndex={1} />);
    expect(getByText('Eşleştirme').closest('li')).toHaveAttribute('aria-current', 'step');
    expect(getAllByRole('listitem').filter((li) => li.hasAttribute('aria-current'))).toHaveLength(1);
    expect(getByText('Adım 2 / 3')).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Stepper steps={steps} currentIndex={0} />);
    await expectNoAxeViolations(container);
  });
});
