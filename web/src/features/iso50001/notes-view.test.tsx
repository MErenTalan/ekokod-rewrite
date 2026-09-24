import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { note } from './_fixture';
import { NotesView, type NotesViewProps } from './notes-view';

const render = (over: Partial<NotesViewProps> = {}) => {
  const props: NotesViewProps = { notes: [note(), note({ id: 'n-2', title: undefined, body: 'Başlıksız içerik' })], editable: true, saving: false,
    onAdd: vi.fn(), onUpdate: vi.fn(), onDelete: vi.fn(), ...over };
  return { props, r: renderWithProviders(<NotesView {...props} />) };
};

describe('NotesView', () => {
  it('adds a titled note (R346)', async () => {
    const { r, props } = render({ notes: [] });
    expect(r.getByText('Henüz not eklenmedi.')).toBeVisible();
    await r.user.type(r.getByRole('textbox', { name: 'Başlık' }), 'Politika');
    await r.user.type(r.getByRole('textbox', { name: /^Not/ }), 'Yayımlandı');
    await r.user.click(r.getByRole('button', { name: 'Notu Listeye Ekle' }));
    expect(props.onAdd).toHaveBeenCalledWith({ title: 'Politika', body: 'Yayımlandı' });
  });

  it('edits in place, cancels, and updates', async () => {
    const { r, props } = render();
    await r.user.click(r.getAllByRole('button', { name: 'Düzenle' })[0]);
    const body = r.getByRole('textbox', { name: /^Not/ });
    await r.user.clear(body);
    await r.user.type(body, 'Güncel');
    await r.user.click(r.getByRole('button', { name: 'Güncelle' }));
    expect(props.onUpdate).toHaveBeenCalledWith(expect.objectContaining({ id: 'n-1' }), { title: 'Politika', body: 'Güncel' });
    await r.user.click(r.getAllByRole('button', { name: 'Düzenle' })[0]);
    await r.user.click(r.getByRole('button', { name: 'Vazgeç' }));
    expect(r.queryByRole('button', { name: 'Güncelle' })).toBeNull();
  });

  it('asks before deleting', async () => {
    const { r, props } = render();
    await r.user.click(r.getAllByRole('button', { name: 'Sil' })[0]);
    expect(await r.findByRole('dialog', { name: 'Not silinsin mi?' })).toBeVisible();
    expect(props.onDelete).not.toHaveBeenCalled();
    await r.user.click(r.getAllByRole('button', { name: 'Sil' }).at(-1)!);
    expect(props.onDelete).toHaveBeenCalledWith(expect.objectContaining({ id: 'n-1' }));
  });

  it('lists notes read-only without the edit right', () => {
    const { r } = render({ editable: false });
    expect(r.getByText('Enerji politikası yayımlandı.')).toBeVisible();
    expect(r.getByText('Başlıksız not')).toBeVisible();
    expect(r.queryByRole('button', { name: 'Düzenle' })).toBeNull();
    expect(r.queryByRole('button', { name: 'Notu Listeye Ekle' })).toBeNull();
  });
});
