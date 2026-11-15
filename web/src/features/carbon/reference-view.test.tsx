import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { catalogue } from './_fixture';
import { ReferenceView } from './reference-view';

describe('ReferenceView', () => {
  it('explains the GHG scopes and maps every activity from the catalogue (R327)', () => {
    const r = renderWithProviders(<ReferenceView kind="ghg" catalogue={catalogue} />);
    expect(r.getByRole('heading', { name: 'GHG Protokolü' })).toBeVisible();
    const waste = r.getByRole('row', { name: /Atık Bertarafı/ });
    expect(waste).toHaveTextContent('Kapsam 3');
    // The GHG page classifies by scope only; ISO categories belong to their own page.
    expect(waste).not.toHaveTextContent('Kategori');
    expect(r.queryByRole('columnheader', { name: 'ISO 14064 kategorisi' })).toBeNull();
    const scopes = r.getAllByRole('row').slice(1).map((row) => row.querySelector('td')?.textContent);
    expect(scopes).toEqual([...scopes].sort());
  });

  it('lists the six ISO 14064 categories and maps activities by category only', () => {
    const r = renderWithProviders(<ReferenceView kind="iso" catalogue={catalogue} />);
    expect(r.getByText('Kategori 6 — Diğer kaynaklardan dolaylı emisyonlar')).toBeVisible();
    const waste = r.getByRole('row', { name: /Atık Bertarafı/ });
    expect(waste).toHaveTextContent('Kategori 6');
    expect(waste).not.toHaveTextContent('Kapsam');
    expect(r.queryByRole('columnheader', { name: 'GHG kapsamı' })).toBeNull();
  });

  it('compares the standards', () => {
    const r = renderWithProviders(<ReferenceView kind="standards" catalogue={catalogue} />);
    expect(r.getByRole('row', { name: /Sınıflandırma/ })).toHaveTextContent('Altı kategori');
  });
});
