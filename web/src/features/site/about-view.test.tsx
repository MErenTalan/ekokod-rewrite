import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { AboutView } from './about-view';

describe('AboutView', () => {
  it('has the §7.19 about sections', () => {
    const { getByRole, getByText } = renderWithProviders(<AboutView />);
    expect(getByRole('heading', { level: 1, name: 'EkoKod Hakkında' })).toBeInTheDocument();
    for (const name of ['Misyonumuz', 'EKO-RM yazılımı', 'İş ortaklarımız', 'Vizyonumuz', 'Sürdürülebilirlik yolculuğumuz', 'Rakamlarla EkoKod', 'Yönetim'])
      expect(getByRole('heading', { name })).toBeInTheDocument();
    expect(getByText('Selçuk Üniversitesi, Konya')).toBeInTheDocument();
    expect(getByText('2020')).toBeInTheDocument();
    expect(getByText('Doç. Dr. Gül Nihal Güğül')).toBeInTheDocument();
  });

  it('numbers the process steps in order', () => {
    const { getAllByRole } = renderWithProviders(<AboutView />);
    const steps = getAllByRole('listitem').filter((li) => li.closest('ol'));
    expect(steps.map((s) => s.querySelector('h3')?.textContent)).toEqual(['Ölç', 'Analiz et', 'Harekete geç', 'Raporla']);
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<AboutView />);
    await expectNoAxeViolations(container);
  });
});
