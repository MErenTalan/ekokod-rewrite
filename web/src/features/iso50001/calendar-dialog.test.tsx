import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { clauses, emptyProject, project } from './_fixture';
import { CalendarDialog } from './calendar-dialog';

describe('CalendarDialog', () => {
  it('saves every main clause with its dates (R344)', async () => {
    const onSave = vi.fn();
    const r = renderWithProviders(<CalendarDialog open project={project} clauses={clauses} saving={false} onSave={onSave} onClose={vi.fn()} />);
    expect(r.getByRole('dialog', { name: 'Proje Tarihlerini Belirle' })).toBeVisible();
    await r.user.click(r.getByRole('button', { name: 'Tarihleri Kaydet' }));
    expect(onSave).toHaveBeenCalledWith(project.clauses.map((c) => ({ clause_id: c.clause_id, start: c.start, end: c.end })));
  });

  it('needs every clause dated before saving', () => {
    const r = renderWithProviders(<CalendarDialog open project={emptyProject} clauses={clauses} saving={false} onSave={vi.fn()} onClose={vi.fn()} />);
    expect(r.getByText('Lütfen her başlık için başlangıç ve bitiş tarihlerini seçin.')).toBeVisible();
    expect(r.getByRole('button', { name: 'Tarihleri Kaydet' })).toBeDisabled();
  });

  it('shows the server refusal', () => {
    const r = renderWithProviders(<CalendarDialog open project={project} clauses={clauses} saving={false} error="Başlangıç tarihi bitiş tarihinden sonra olamaz." onSave={vi.fn()} onClose={vi.fn()} />);
    expect(r.getByText('Başlangıç tarihi bitiş tarihinden sonra olamaz.')).toBeVisible();
  });
});
