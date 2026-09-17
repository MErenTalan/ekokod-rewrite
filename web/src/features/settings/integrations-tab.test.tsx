import { describe, expect, it, vi } from 'vitest';

import type { IntegrationDefinition } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { IntegrationsTabView } from './integrations-tab';

const definitions: IntegrationDefinition[] = [
  {
    id: 'd-1',
    provider: 'osos',
    subtype: 'Baskent',
    endpoints: { authentication: 'https://api.ornek/auth', analyzer_list: 'https://api.ornek/list' },
    updated_at: '2026-09-01T09:00:00+03:00',
  },
];

describe('IntegrationsTabView', () => {
  it('lists each definition with how many endpoints it declares', () => {
    const r = renderWithProviders(<IntegrationsTabView definitions={definitions} onSave={() => {}} onDelete={() => {}} />);
    expect(r.getByRole('row', { name: /osos/ })).toHaveTextContent('2 uç nokta');
  });

  it('refuses an endpoint that is not https (R176)', async () => {
    const onSave = vi.fn();
    const r = renderWithProviders(<IntegrationsTabView definitions={definitions} onSave={onSave} onDelete={() => {}} />);
    await r.user.click(r.getByRole('button', { name: 'Tanımı düzenle' }));
    const url = (await r.findAllByLabelText(/Adres/))[0];
    await r.user.clear(url);
    await r.user.type(url, 'http://api.ornek/auth');
    expect(r.getByText('Adres https:// ile başlamalı')).toBeInTheDocument();
    expect(r.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
  });

  it('saves a new definition with its endpoints', async () => {
    const onSave = vi.fn();
    const r = renderWithProviders(<IntegrationsTabView definitions={[]} onSave={onSave} onDelete={() => {}} />);
    await r.user.click(r.getByRole('button', { name: 'Tanım ekle' }));
    await r.user.type(await r.findByLabelText(/Alt tür/), 'Aydem');
    await r.user.click(r.getByRole('button', { name: 'Uç nokta ekle' }));
    await r.user.type((await r.findAllByLabelText(/Adres/))[0], 'https://api.aydem/auth');
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ provider: 'osos', subtype: 'Aydem', endpoints: [{ key: 'authentication', url: 'https://api.aydem/auth' }] }),
    );
  });

  it('asks before deleting', async () => {
    const onDelete = vi.fn();
    const r = renderWithProviders(<IntegrationsTabView definitions={definitions} onSave={() => {}} onDelete={onDelete} />);
    await r.user.click(r.getByRole('button', { name: 'Tanımı sil' }));
    expect(await r.findByText('osos/Baskent tanımı silinsin mi?')).toBeInTheDocument();
    await r.user.click(r.getByRole('button', { name: 'Sil' }));
    expect(onDelete).toHaveBeenCalledWith(definitions[0]);
  });
});
