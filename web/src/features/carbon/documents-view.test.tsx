import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { DocumentsView } from './documents-view';

describe('DocumentsView', () => {
  it('says plainly that storage is not available yet (Q-F9)', () => {
    const r = renderWithProviders(<DocumentsView />);
    expect(r.getByText('Belge deposu henüz kullanılamıyor')).toBeVisible();
    expect(r.queryByRole('button')).toBeNull();
  });
});
