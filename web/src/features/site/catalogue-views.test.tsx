import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { DocumentsView, ReferencesView, ToolkitView } from './catalogue-views';

describe('catalogue views', () => {
  it('references tell each success story and invite a demo', () => {
    const { getByRole, getAllByRole } = renderWithProviders(<ReferencesView />);
    expect(getByRole('heading', { level: 1, name: 'Referanslarımız' })).toBeInTheDocument();
    expect(getAllByRole('heading', { level: 2 })).toHaveLength(4);
    expect(getByRole('link', { name: /Demo talep et/ })).toHaveAttribute('href', '/request-demo');
  });

  it('documents are grouped and each can be requested', () => {
    const { getAllByRole, getByRole } = renderWithProviders(<DocumentsView />);
    expect(getByRole('heading', { level: 1, name: 'Dokümanlar' })).toBeInTheDocument();
    expect(getAllByRole('heading', { level: 2 }).map((h) => h.textContent)).toEqual(['Genel', 'Teknik dokümanlar', 'Eğitim materyalleri']);
    expect(getAllByRole('heading', { level: 3 })).toHaveLength(7);
  });

  it('the toolkit lists both products', () => {
    const { getByRole } = renderWithProviders(<ToolkitView />);
    expect(getByRole('heading', { level: 1, name: 'Yazılım Çözümleri' })).toBeInTheDocument();
    expect(getByRole('heading', { name: 'Kaynak Yönetim Yazılımı (EKO-RM)' })).toBeInTheDocument();
    expect(getByRole('heading', { name: 'Karbon Yönetim Yazılımı (EKO-CM)' })).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    for (const View of [ReferencesView, DocumentsView, ToolkitView]) {
      const { container, unmount } = renderWithProviders(<View />);
      await expectNoAxeViolations(container);
      unmount();
    }
  });
});
