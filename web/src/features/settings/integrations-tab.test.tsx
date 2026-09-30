import { describe, expect, it, vi } from 'vitest';

import type { IntegrationDefinition } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { IntegrationsTabView, type ProviderEndpointKeys } from './integrations-tab';

const definitions: IntegrationDefinition[] = [
  {
    id: 'd-1',
    provider: 'osos',
    subtype: 'Baskent',
    endpoints: {
      authentication: 'https://api.ornek/auth',
      analyzers_list: 'https://api.ornek/list',
    },
    updated_at: '2026-09-01T09:00:00+03:00',
  },
];

const catalog: ProviderEndpointKeys[] = [
  { provider: 'osos', required: ['authentication', 'analyzers_list'], optional: [] },
  {
    provider: 'gridbox',
    required: ['token', 'last_endex', 'endexes'],
    optional: ['energy_values'],
  },
];

describe('IntegrationsTabView', () => {
  it('lists each definition with how many endpoints it declares', () => {
    const r = renderWithProviders(
      <IntegrationsTabView definitions={definitions} onSave={() => {}} onDelete={() => {}} />,
    );
    expect(r.getByRole('row', { name: /osos/ })).toHaveTextContent('2 uç nokta');
  });

  it('refuses an endpoint that is not https (R176)', async () => {
    const onSave = vi.fn();
    const r = renderWithProviders(
      <IntegrationsTabView definitions={definitions} onSave={onSave} onDelete={() => {}} />,
    );
    await r.user.click(r.getByRole('button', { name: 'Tanımı düzenle' }));
    const url = (await r.findAllByLabelText(/Adres/))[0];
    await r.user.clear(url);
    await r.user.type(url, 'http://api.ornek/auth');
    expect(r.getByText('Adres https:// ile başlamalı')).toBeInTheDocument();
    expect(r.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
  });

  it('saves a new definition with its endpoints', async () => {
    const onSave = vi.fn();
    const r = renderWithProviders(
      <IntegrationsTabView
        definitions={[]}
        endpointKeys={catalog}
        onSave={onSave}
        onDelete={() => {}}
      />,
    );
    await r.user.click(r.getByRole('button', { name: 'Tanım ekle' }));
    await r.user.type(await r.findByLabelText(/Alt tür/), 'Aydem');
    const urls = await r.findAllByLabelText(/Adres/);
    expect(urls).toHaveLength(2);
    await r.user.type(urls[0], 'https://api.aydem/auth');
    expect(r.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
    await r.user.type(urls[1], 'https://api.aydem/list');
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        provider: 'osos',
        subtype: 'Aydem',
        endpoints: [
          { key: 'authentication', url: 'https://api.aydem/auth' },
          { key: 'analyzers_list', url: 'https://api.aydem/list' },
        ],
      }),
    );
  });

  it('flags a definition whose keys the adapter does not read', async () => {
    const legacy: IntegrationDefinition = {
      id: 'd-2',
      provider: 'gridbox',
      subtype: 'meramedas',
      endpoints: { token: 'https://x/t', last_index: 'https://x/e', indexes: 'https://x/i' },
      updated_at: '2026-09-01T09:00:00+03:00',
    };
    const r = renderWithProviders(
      <IntegrationsTabView
        definitions={[legacy]}
        endpointKeys={catalog}
        onSave={() => {}}
        onDelete={() => {}}
      />,
    );
    expect(r.getByRole('row', { name: /gridbox/ })).toHaveTextContent(
      'Eksik zorunlu uç nokta: last_endex, endexes',
    );
    await r.user.click(r.getByRole('button', { name: 'Tanımı düzenle' }));
    expect((await r.findAllByText('Bu sağlayıcı bu anahtarı kullanmıyor')).length).toBe(2);
    expect(r.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
  });

  it('asks before deleting', async () => {
    const onDelete = vi.fn();
    const r = renderWithProviders(
      <IntegrationsTabView definitions={definitions} onSave={() => {}} onDelete={onDelete} />,
    );
    await r.user.click(r.getByRole('button', { name: 'Tanımı sil' }));
    expect(await r.findByText('osos/Baskent tanımı silinsin mi?')).toBeInTheDocument();
    await r.user.click(r.getByRole('button', { name: 'Sil' }));
    expect(onDelete).toHaveBeenCalledWith(definitions[0]);
  });
});
