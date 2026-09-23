import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';

import { renderWithProviders } from '@/test/render';

import { demoBuildingStates, demoTemplates } from './_fixture';
import { TemplatesTabView } from './templates-tab';

const view = (over: Partial<Parameters<typeof TemplatesTabView>[0]> = {}) => {
  const onApply = vi.fn();
  const r = renderWithProviders(
    <TemplatesTabView
      templates={demoTemplates}
      buildings={demoBuildingStates}
      canEdit
      onCreate={() => {}}
      onEdit={() => {}}
      onDelete={() => {}}
      onApply={onApply}
      {...over}
    />,
  );
  return { ...r, onApply };
};

describe('TemplatesTabView', () => {
  it('lists templates and marks the default one', () => {
    const r = view();
    // The name cell also carries the "default" badge, so match on content.
    expect(r.getByRole('cell', { name: /Ticari AG/ })).toBeVisible();
    expect(r.getByText('Varsayılan')).toBeVisible();
  });

  it('applies a template to the buildings the operator checked, and to no others', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('button', { name: /Binalara uygula/ }));
    await user.click(r.getByRole('checkbox', { name: /A1 Fabrika/ }));
    await user.type(r.getByLabelText(/Yürürlük tarihi/), '2026-10-01');
    await user.click(r.getByRole('button', { name: /^Uygula$/ }));

    expect(r.onApply).toHaveBeenCalledWith({ templateID: 'tpl-1', buildingIDs: ['b-1'], effectiveFrom: '2026-10-01' });
  });

  it('refuses to apply with nothing selected', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('button', { name: /Binalara uygula/ }));
    await user.type(r.getByLabelText(/Yürürlük tarihi/), '2026-10-01');
    await user.click(r.getByRole('button', { name: /^Uygula$/ }));

    expect(r.onApply).not.toHaveBeenCalled();
    expect(await r.findByRole('alert')).toHaveTextContent(/En az bir bina seçin/);
  });

  it('offers no write action to a principal who may only read templates', () => {
    const r = view({ canEdit: false });
    expect(r.queryByRole('button', { name: /Yeni şablon/ })).toBeNull();
    expect(r.queryByRole('button', { name: /Binalara uygula/ })).toBeNull();
  });

  it('shows an empty state when there is no template', () => {
    expect(view({ templates: [] }).getByText(/Henüz şablon yok/)).toBeVisible();
  });
});
