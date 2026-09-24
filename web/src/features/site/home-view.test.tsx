import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { HomeView } from './home-view';

describe('HomeView', () => {
  it('opens with the hero and its two calls to action', () => {
    const { getByRole, getAllByRole } = renderWithProviders(<HomeView pricing={false} />);
    expect(getByRole('heading', { level: 1 })).toHaveTextContent('Enerji, su ve yakıt tüketiminizi tek yerden yönetin');
    const [demo] = getAllByRole('link', { name: 'Demo talep et' });
    expect(demo).toHaveAttribute('href', '/request-demo');
    expect(getByRole('link', { name: 'Faturanı hesapla' })).toHaveAttribute('href', '/bill-calculator');
  });

  it('carries the §7.19 sections, genuine content only', () => {
    const { getByRole, queryByText } = renderWithProviders(<HomeView pricing={false} />);
    for (const name of ['EKO-RM ile neler yapabilirsiniz?', 'EKO-CM ile neler yapabilirsiniz?', 'Referanslarımız', 'Dokümanlar', 'Haberler', 'Yazılım çözümlerimiz', 'Yönetim', 'Sık sorulan sorular'])
      expect(getByRole('heading', { name, level: 2 })).toBeInTheDocument();
    expect(queryByText(/Alex Martinez|Discord|Ahmet Yılmaz/)).toBeNull();
  });

  it('teases pricing only behind its flag', () => {
    const { queryByRole, getByRole, unmount } = renderWithProviders(<HomeView pricing={false} />);
    expect(queryByRole('link', { name: /Paketleri karşılaştır/ })).toBeNull();
    unmount();
    renderWithProviders(<HomeView pricing />);
    expect(getByRole('link', { name: /Paketleri karşılaştır/ })).toHaveAttribute('href', '/pricing');
  });

  it('answers the FAQ on demand', async () => {
    const { getByRole, findByText, user } = renderWithProviders(<HomeView pricing={false} />);
    await user.click(getByRole('button', { name: 'Kendi sunucularımıza kurabilir miyiz?' }));
    expect(await findByText(/kurum içi sunucularınızda/)).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<HomeView pricing />);
    await expectNoAxeViolations(container);
  });
});
