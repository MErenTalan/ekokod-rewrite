import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { clauses, emptyProject, project } from './_fixture';
import { SummaryView, type SummaryViewProps } from './summary-view';

const render = (over: Partial<SummaryViewProps> = {}) => {
  const props: SummaryViewProps = { project, clauses, today: '2026-06-15', canEdit: true, downloading: false, onDownload: vi.fn(), onCalendar: vi.fn(), ...over };
  return { props, r: renderWithProviders(<SummaryView {...props} />) };
};

describe('SummaryView', () => {
  it('shows the progress with the counted sub-clauses (R343)', () => {
    const { r } = render();
    expect(r.getByRole('progressbar', { name: 'Proje İlerlemesi' })).toHaveAttribute('aria-valuenow', '15');
    expect(r.getByText('Tamamlanan: 2 / 20 alt madde')).toBeVisible();
  });

  it('lists each sub-clause with its notes and files, or legacy messages when empty', () => {
    const { r } = render();
    const first = r.getByRole('listitem', { name: '5.1 Liderlik ve Taahhüt' });
    expect(first).toHaveTextContent('2 not');
    expect(first).toHaveTextContent('Dosya yüklenmedi.');
    const second = r.getByRole('listitem', { name: '5.2 Enerji Politikası' });
    expect(second).toHaveTextContent('Not eklenmedi.');
    expect(second).toHaveTextContent('1 dosya');
  });

  it('downloads the folder and opens the calendar', async () => {
    const { r, props } = render();
    await r.user.click(r.getByRole('button', { name: 'ISO 50001 Klasörünü İndir' }));
    expect(props.onDownload).toHaveBeenCalled();
    await r.user.click(r.getByRole('button', { name: 'Takvimi Güncelle' }));
    expect(props.onCalendar).toHaveBeenCalled();
  });

  it('offers a viewer no calendar and asks an editor to set dates first', () => {
    const viewer = render({ canEdit: false });
    expect(viewer.r.queryByRole('button', { name: /Takvimi|Tarihleri/ })).toBeNull();
    viewer.r.unmount();
    const editor = render({ project: emptyProject });
    expect(editor.r.getByRole('button', { name: 'Tarihleri Ayarla' })).toBeVisible();
  });
});
