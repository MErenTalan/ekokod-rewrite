import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { ReactiveStatusCard } from './reactive-status-card';

const base = { period: 'Ağustos 2026', penaltyApplied: true };

describe('ReactiveStatusCard', () => {
  it('over the limit is danger with text', () => {
    const { getByText, getAllByRole } = renderWithProviders(
      <ReactiveStatusCard {...base} inductive={{ ratio: '0.25', limit: '0.20' }} capacitive={{ ratio: '0.05', limit: '0.15' }} />,
    );
    expect(getByText('Sınır aşıldı')).toBeInTheDocument();
    expect(getByText('Sınır içinde')).toBeInTheDocument();
    expect(getByText('%25')).toBeInTheDocument();
    expect(getAllByRole('progressbar')[0]).toHaveAttribute('aria-valuetext', '%25');
    expect(getByText('Reaktif ceza uygulandı')).toBeInTheDocument();
  });

  it('a measured ratio is shown to two decimals, not as the API sent it', () => {
    const { getByText, queryByText } = renderWithProviders(
      <ReactiveStatusCard {...base} inductive={{ ratio: '0.000085781276806649', limit: '0.20' }} capacitive={{ ratio: '0.37501667969271240397', limit: '0.15' }} />,
    );
    expect(getByText('%37,5')).toBeInTheDocument();
    expect(getByText('%0,01')).toBeInTheDocument();
    expect(queryByText(/501667/)).toBeNull();
  });

  it('null ratio is no-data, never zero', () => {
    const { getAllByText, container } = renderWithProviders(
      <ReactiveStatusCard {...base} penaltyApplied={null} inductive={{ ratio: null, limit: '0.20' }} capacitive={{ ratio: null, limit: '0.15' }} />,
    );
    expect(getAllByText('Veri yok').length).toBeGreaterThan(0);
    expect(container.textContent).not.toContain('%0');
  });

  it('exactly at the limit is within the limit', () => {
    const { getAllByText } = renderWithProviders(<ReactiveStatusCard {...base} inductive={{ ratio: '0.20', limit: '0.2' }} capacitive={{ ratio: '0.15', limit: '0.15' }} />);
    expect(getAllByText('Sınır içinde')).toHaveLength(2);
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<ReactiveStatusCard {...base} advisory="Kompanzasyon panosunu kontrol edin." inductive={{ ratio: '0.25', limit: '0.20' }} capacitive={{ ratio: '0.05', limit: '0.15' }} />);
    await expectNoAxeViolations(container);
  });
});
