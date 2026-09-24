import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { ReactivePanelView, type ReactiveRow } from './reactive-panel';

const over: ReactiveRow = {
  analyzer_id: 'a-1',
  building_id: 'b-1',
  analyzerLabel: 'Merkez Ofis Analizör',
  buildingName: 'A1 Fabrika',
  has_data: true,
  inductive_ratio: '0.3512',
  capacitive_ratio: '0.0421',
  inductive_limit: '0.20',
  capacitive_limit: '0.15',
  penalty_applies: true,
};
const exempt: ReactiveRow = {
  ...over,
  analyzer_id: 'a-2',
  building_id: 'b-2',
  analyzerLabel: '4009876543',
  buildingName: 'A2 Depo',
  inductive_ratio: '0.1102',
  penalty_applies: false,
  exempt_reason: 'below_kw',
};

const view = (overrides: Partial<React.ComponentProps<typeof ReactivePanelView>> = {}) => (
  <ReactivePanelView
    month="2026-09"
    maxMonth="2026-09"
    onMonthChange={() => {}}
    scope="all"
    scopeOptions={[
      { value: 'all', label: 'Tüm binalar' },
      { value: 'b-1', label: 'A1 Fabrika' },
    ]}
    onScopeChange={() => {}}
    rows={[over, exempt]}
    highestInductive={over}
    highestCapacitive={over}
    {...overrides}
  />
);

describe('ReactivePanelView', () => {
  it('advises correction when any analyzer is over its limit, and names the worst one', () => {
    const r = renderWithProviders(view());
    expect(r.getByText('Kompanzasyon (güç faktörü düzeltmesi) gerekiyor.')).toBeInTheDocument();
    expect(r.getByText('En yüksek endüktif: Merkez Ofis Analizör · A1 Fabrika')).toBeInTheDocument();
    expect(r.getByText('%35,12')).toBeInTheDocument();
    expect(r.getByText('Sınır aşıldı')).toBeInTheDocument();
  });

  it('advises correction even when only a later analyzer is over its limit', () => {
    const r = renderWithProviders(view({ rows: [exempt, over], highestInductive: over, highestCapacitive: over }));
    expect(r.getByText('Kompanzasyon (güç faktörü düzeltmesi) gerekiyor.')).toBeInTheDocument();
  });

  it('reports limits kept when nobody is over', () => {
    const r = renderWithProviders(view({ rows: [exempt], highestInductive: exempt, highestCapacitive: exempt }));
    expect(r.getByText('Oranlar sınırlar içinde.')).toBeInTheDocument();
  });

  it('lists every analyzer with its exemption in the dialog', async () => {
    const r = renderWithProviders(view());
    await r.user.click(r.getByRole('button', { name: 'Tüm binalar' }));
    const dialog = await r.findByRole('dialog');
    expect(dialog).toHaveTextContent('A2 Depo');
    expect(dialog).toHaveTextContent('Kurulu güç sınırın altında — muaf');
    expect(r.getAllByRole('link', { name: 'Faturayı gör' })[0]).toHaveAttribute(
      'href',
      '/ekorm/bills?building_id=b-1&period=2026-09',
    );
  });

  it('says there is nothing to show for an empty month', () => {
    const r = renderWithProviders(view({ rows: [], highestInductive: undefined, highestCapacitive: undefined }));
    expect(r.getByText('Seçilen ay için reaktif verisi yok')).toBeInTheDocument();
    // Nothing to list: the all-analyzers dialog button is left out rather than shown disabled.
    expect(r.queryByRole('button', { name: 'Tüm binalar' })).toBeNull();
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(view());
    await expectNoAxeViolations(r.container);
  });
});
