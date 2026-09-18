import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoMessages } from './_fixture';
import { MessageTableView } from './message-table';

describe('MessageTableView', () => {
  it('shows every §7.13 column, including the related entity', () => {
    const r = renderWithProviders(<MessageTableView messages={demoMessages} filtered={false} />);
    expect(r.getByText('Alarm tetiklendi')).toBeVisible();
    expect(r.getByText(/Endüktif oran/)).toBeVisible();
    expect(r.getByText('Uyarı')).toBeVisible();
    // The related entity is a column of its own, not a hint inside the message.
    expect(r.getAllByRole('columnheader', { name: 'İlgili Kayıt' })).toHaveLength(1);
  });

  it('names all three kinds', () => {
    const r = renderWithProviders(<MessageTableView messages={demoMessages} filtered={false} />);
    expect(r.getByText('Alarmlar')).toBeVisible();
    expect(r.getByText('Otomatik İşlemler')).toBeVisible();
    expect(r.getByText('Sistem')).toBeVisible();
  });

  it('renders a message with no detail and no relation as an em dash', () => {
    const r = renderWithProviders(<MessageTableView messages={[demoMessages[1]]} filtered={false} />);
    expect(r.getAllByText('—')).toHaveLength(2);
  });

  it('says whether the list is empty or merely filtered', () => {
    const empty = renderWithProviders(<MessageTableView messages={[]} filtered={false} />);
    expect(empty.getByText('Henüz mesaj yok.')).toBeVisible();
    const filtered = renderWithProviders(<MessageTableView messages={[]} filtered />);
    expect(filtered.getByText('Filtre kriterlerine uygun mesaj bulunamadı.')).toBeVisible();
  });
});
