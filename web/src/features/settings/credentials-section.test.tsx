import { describe, expect, it, vi } from 'vitest';

import type { Building, IntegrationCredential, IntegrationDefinition } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { CredentialsSectionView } from './credentials-section';

const credentials: IntegrationCredential[] = [
  {
    id: 'c-1',
    definition_id: 'd-1',
    provider: 'osos',
    subtype: 'Baskent',
    username: 'osos-user',
    has_secret: true,
    extra_keys: [],
    is_active: true,
    last_verified_at: '2026-09-15T09:00:00+03:00',
    updated_at: '2026-09-15T09:00:00+03:00',
  },
  {
    id: 'c-2',
    definition_id: 'd-4',
    provider: 'isolar',
    subtype: 'eu',
    has_secret: false,
    extra_keys: ['app_key'],
    is_active: true,
    last_verified_at: null,
    updated_at: '2026-09-15T09:00:00+03:00',
  },
];

const view = (overrides: Partial<React.ComponentProps<typeof CredentialsSectionView>> = {}) => (
  <CredentialsSectionView
    credentials={credentials}
    definitions={[] as IntegrationDefinition[]}
    buildings={[] as Building[]}
    canCreate
    today="2026-09-17"
    onSave={() => {}}
    onVerify={() => {}}
    onDiscover={() => {}}
    onBackfill={() => {}}
    onDelete={() => {}}
    onConnectIsolar={() => {}}
    {...overrides}
  />
);

describe('CredentialsSectionView', () => {
  it('says a secret is stored without ever showing one', () => {
    const r = renderWithProviders(view());
    const row = r.getByRole('row', { name: /osos/ });
    expect(row).toHaveTextContent('Kayıtlı');
    expect(row).not.toHaveTextContent('osos-pass');
    expect(r.getByRole('row', { name: /isolar/ })).toHaveTextContent('Hiç');
  });

  it('hides creation from a role that cannot read the definitions (R201)', () => {
    const r = renderWithProviders(view({ canCreate: false }));
    expect(r.queryByRole('button', { name: 'Entegrasyon ekle' })).toBeNull();
  });

  it('starts discovery and shows the job it started', async () => {
    const onDiscover = vi.fn();
    const r = renderWithProviders(
      view({ onDiscover, job: { id: 'job-1', label: 'Analizörleri keşfet', status: 'running' } }),
    );
    await r.user.click(r.getAllByRole('button', { name: /İşlemler/ })[0]);
    await r.user.click(await r.findByRole('menuitem', { name: 'Analizörleri keşfet' }));
    expect(onDiscover).toHaveBeenCalledWith(credentials[0]);
    expect(r.getByText(/Analizörleri keşfet: Çalışıyor/)).toBeInTheDocument();
  });

  it('offers the iSolar connection only for an iSolar credential', async () => {
    const r = renderWithProviders(view());
    await r.user.click(r.getAllByRole('button', { name: /İşlemler/ })[0]);
    expect(r.queryByRole('menuitem', { name: "iSolarCloud'a bağlan" })).toBeNull();
    await r.user.keyboard('{Escape}');
    await r.user.click(r.getAllByRole('button', { name: /İşlemler/ })[1]);
    expect(await r.findByRole('menuitem', { name: "iSolarCloud'a bağlan" })).toBeInTheDocument();
  });

  it('asks for a range before backfilling', async () => {
    const onBackfill = vi.fn();
    const r = renderWithProviders(view({ onBackfill }));
    await r.user.click(r.getAllByRole('button', { name: /İşlemler/ })[0]);
    await r.user.click(await r.findByRole('menuitem', { name: 'Geçmiş veriyi çek' }));
    const dialog = await r.findByRole('dialog');
    expect(dialog).toHaveTextContent('Başlangıç');
    await r.user.click(r.getAllByRole('button', { name: 'Geçmiş veriyi çek' }).at(-1)!);
    expect(onBackfill).toHaveBeenCalledWith(credentials[0], { from: '2026-09-17', to: '2026-09-17' });
  });
});
