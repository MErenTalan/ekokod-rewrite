import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { me } from './_fixture';

const searchParams = vi.hoisted(() => ({ current: new URLSearchParams() }));
vi.mock('next/navigation', async () => {
  const { mockPathname: pathname, mockRouter: router } = await import('@/test/navigation');
  return { usePathname: () => pathname.current, useRouter: () => router, useSearchParams: () => searchParams.current, redirect: vi.fn() };
});

const { IsoPage } = await import('./iso-page');

let api: ReturnType<typeof mockApi>;
beforeEach(() => {
  api = mockApi({ 'GET /api/v1/buildings': { items: [], next_cursor: null }, 'GET /api/v1/analyzers': { items: [], next_cursor: null } });
});
afterEach(() => {
  api.restore();
  searchParams.current = new URLSearchParams();
});

describe('IsoPage', () => {
  it('opens the tab the URL names and shows the guidance footer on every view', async () => {
    searchParams.current = new URLSearchParams('tab=checklist');
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <IsoPage />
      </SessionProvider>,
    );
    expect(await r.findByRole('tab', { name: 'Dosya Yükleme Sayfası', selected: true })).toBeVisible();
    expect(r.getByText('Tüm maddeler TS EN ISO 50001:2018 standardına dayanmaktadır.')).toBeVisible();
    expect(r.getByText('Bu araç yalnızca rehberlik amaçlıdır.')).toBeVisible();
  });

  it('asks for a building first', async () => {
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <IsoPage />
      </SessionProvider>,
    );
    expect(await r.findByText('Önce bir bina seçin')).toBeVisible();
  });

  it('says so when the role cannot read the module', () => {
    const r = renderWithProviders(
      <SessionProvider me={{ ...me('company_admin'), permissions: ['nav.core'] as MeResponse['permissions'] }}>
        <IsoPage />
      </SessionProvider>,
    );
    expect(r.getByText('Bu modüle erişiminiz yok')).toBeVisible();
  });
});
