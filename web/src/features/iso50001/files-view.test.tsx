import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { file } from './_fixture';
import { FilesView, type FilesViewProps } from './files-view';

const render = (over: Partial<FilesViewProps> = {}) => {
  const props: FilesViewProps = { files: [file()], editable: true, uploading: false, onUpload: vi.fn(), onDownload: vi.fn(), onDelete: vi.fn(), ...over };
  return { props, r: renderWithProviders(<FilesView {...props} />) };
};

describe('FilesView', () => {
  it('lists files with size and downloads through the handler (R346)', async () => {
    const { r, props } = render();
    expect(r.getByText('Politika.pdf')).toBeVisible();
    await r.user.click(r.getByRole('button', { name: 'Politika.pdf indir' }));
    expect(props.onDownload).toHaveBeenCalledWith(file());
  });

  it('uploads a chosen file', async () => {
    const { r, props } = render({ files: [] });
    expect(r.getByText('Henüz dosya eklenmedi.')).toBeVisible();
    const pdf = new File(['%PDF-1.7'], 'Kanıt.pdf', { type: 'application/pdf' });
    await r.user.upload(r.getByLabelText(/Gerekli Dosyaları Yükle/), pdf);
    expect(props.onUpload).toHaveBeenCalledWith(pdf);
  });

  it('asks before deleting', async () => {
    const { r, props } = render();
    await r.user.click(r.getByRole('button', { name: 'Politika.pdf sil' }));
    expect(await r.findByRole('dialog', { name: 'Dosya silinsin mi?' })).toBeVisible();
    await r.user.click(r.getByRole('button', { name: 'Sil' }));
    expect(props.onDelete).toHaveBeenCalledWith(file());
  });

  it('shows the server reason and hides writes from viewers', () => {
    const { r } = render({ error: 'Dosya boyutu 30 MB sınırını aşamaz.' });
    expect(r.getByText('Dosya boyutu 30 MB sınırını aşamaz.')).toBeVisible();
    r.unmount();
    const viewer = render({ editable: false });
    expect(viewer.r.queryByRole('button', { name: 'Politika.pdf sil' })).toBeNull();
    expect(viewer.r.queryByLabelText(/Gerekli Dosyaları Yükle/)).toBeNull();
  });
});
