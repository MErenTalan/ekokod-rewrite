import { describe, expect, it, vi } from 'vitest';

import type { Company } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { CompanyListView } from './company-list';

const companies: Company[] = [
  { id: 'c-own', name: 'Ekokod Platform', sector: 'Hizmet', created_at: '', updated_at: '' },
  { id: 'c-a', name: 'Anadolu Tekstil', sector: 'Üretim', created_at: '', updated_at: '' },
];

const view = (overrides: Partial<React.ComponentProps<typeof CompanyListView>> = {}) => (
  <CompanyListView
    companies={companies}
    ownCompanyId="c-own"
    hasMore={false}
    onLoadMore={() => {}}
    onActAs={() => {}}
    onCreate={() => {}}
    onDelete={() => {}}
    {...overrides}
  />
);

describe('CompanyListView', () => {
  it('cannot delete the admin own company', () => {
    const r = renderWithProviders(view());
    const own = r.getByRole('row', { name: /Ekokod Platform/ });
    expect(own.querySelector('button[disabled]')).not.toBeNull();
  });

  it('acts for another company and reports the active one', async () => {
    const onActAs = vi.fn();
    const r = renderWithProviders(view({ onActAs, activeCompanyId: 'c-a' }));
    expect(r.getByRole('row', { name: /Anadolu Tekstil/ })).toHaveTextContent('Şu an bu şirket için çalışıyorsunuz');
    await r.user.click(r.getAllByRole('button', { name: 'Bu şirket adına çalış' })[0]);
    // Choosing the own company clears the override rather than scoping to itself.
    expect(onActAs).toHaveBeenCalledWith(undefined);
  });

  it('loads more only when there is more', () => {
    const none = renderWithProviders(view());
    expect(none.queryByRole('button', { name: 'Daha fazla yükle' })).toBeNull();
    none.unmount();
    const more = renderWithProviders(view({ hasMore: true }));
    expect(more.getByRole('button', { name: 'Daha fazla yükle' })).toBeInTheDocument();
  });

  it('creates a company and confirms a delete', async () => {
    const onCreate = vi.fn();
    const onDelete = vi.fn();
    const r = renderWithProviders(view({ onCreate, onDelete }));
    await r.user.click(r.getByRole('button', { name: 'Şirket ekle' }));
    await r.user.type(await r.findByLabelText(/Şirket adı/), 'Yeni Müşteri');
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    expect(onCreate).toHaveBeenCalledWith({ name: 'Yeni Müşteri' });

    // The own company's delete is disabled, so the second row is the one that asks.
    await r.user.click(r.getAllByRole('button', { name: 'Sil' })[1]);
    expect(await r.findByText('Anadolu Tekstil şirketi silinsin mi?')).toBeInTheDocument();
  });
});
