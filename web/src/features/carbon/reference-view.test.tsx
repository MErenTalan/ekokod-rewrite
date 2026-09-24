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
    expect(waste).toHaveTextContent('Kategori 6');
  });

  it('lists the six ISO 14064 categories', () => {
    const r = renderWithProviders(<ReferenceView kind="iso" catalogue={catalogue} />);
    expect(r.getByText('Kategori 6 — Diğer kaynaklardan dolaylı emisyonlar')).toBeVisible();
  });

  it('compares the standards', () => {
    const r = renderWithProviders(<ReferenceView kind="standards" catalogue={catalogue} />);
    expect(r.getByRole('row', { name: /Sınıflandırma/ })).toHaveTextContent('Altı kategori');
  });
});
