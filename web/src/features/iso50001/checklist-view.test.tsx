import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { clauses } from './_fixture';
import { ChecklistView } from './checklist-view';

const templates = [{ id: 'significant-energy-uses', file_name: 'Onemli-Enerji-Kullanimlari.xlsx', clauses: ['6.3'], description: 'Pareto analizi.' }];

describe('ChecklistView', () => {
  it('shows each main clause, and inside it every sub-clause with its full text (R345)', async () => {
    const r = renderWithProviders(<ChecklistView clauses={clauses} templates={templates} onTemplate={vi.fn()} renderSub={(s) => <p>{`içerik ${s.id}`}</p>} />);
    await r.user.click(r.getByRole('button', { name: '5. Liderlik' }));
    expect(r.getByRole('heading', { name: '5.1 Liderlik ve Taahhüt' })).toBeVisible();
    expect(r.getByText('Üst yönetim, EnYS’nin etkililiği için liderlik göstermelidir.')).toBeVisible();
    expect(r.getByText('içerik 5.2')).toBeVisible();
  });

  it('offers the clause template', async () => {
    const onTemplate = vi.fn();
    const r = renderWithProviders(<ChecklistView clauses={clauses} templates={templates} onTemplate={onTemplate} renderSub={() => null} />);
    await r.user.click(r.getByRole('button', { name: '6. Planlama' }));
    expect(r.getByText('Pareto analizi.')).toBeVisible();
    await r.user.click(r.getByRole('button', { name: 'Onemli-Enerji-Kullanimlari.xlsx indir' }));
    expect(onTemplate).toHaveBeenCalledWith(templates[0]);
  });
});
