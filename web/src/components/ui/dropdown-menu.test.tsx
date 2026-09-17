import { Download, Trash2 } from 'lucide-react';
import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { DropdownMenu, type DropdownMenuItem } from './dropdown-menu';

function items(onExport = vi.fn()): DropdownMenuItem[] {
  return [
    { type: 'label', label: 'Dosya' },
    { type: 'item', label: 'CSV indir', icon: Download, onSelect: onExport },
    { type: 'item', label: 'Excel', description: 'Yakında', onSelect: () => {}, disabled: true },
    { type: 'separator' },
    { type: 'item', label: 'Sil', icon: Trash2, tone: 'danger', onSelect: () => {} },
  ];
}

describe('DropdownMenu', () => {
  it('arrow and Enter select', async () => {
    const onExport = vi.fn();
    const { getByRole, findByRole, user } = renderWithProviders(<DropdownMenu trigger={<button type="button">İşlemler</button>} items={items(onExport)} />);
    getByRole('button', { name: 'İşlemler' }).focus();
    await user.keyboard('{Enter}');
    await findByRole('menu');
    await user.keyboard('{Enter}');
    expect(onExport).toHaveBeenCalled();
  });

  it('disabled items describe why', async () => {
    const { getByRole, findByRole, user } = renderWithProviders(<DropdownMenu trigger={<button type="button">İşlemler</button>} items={items()} />);
    await user.click(getByRole('button', { name: 'İşlemler' }));
    const excel = await findByRole('menuitem', { name: /Excel/ });
    expect(excel).toHaveAttribute('aria-disabled', 'true');
    expect(excel).toHaveAccessibleDescription('Yakında');
  });

  it('has no axe violations when open', async () => {
    const { baseElement, getByRole, findByRole, user } = renderWithProviders(<DropdownMenu trigger={<button type="button">İşlemler</button>} items={items()} />);
    await user.click(getByRole('button', { name: 'İşlemler' }));
    await findByRole('menu');
    await expectNoAxeViolations(baseElement);
  });
});
