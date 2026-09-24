import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';
import { DEFAULT_EVENT_COLOUR } from '@/styles/event-palette';

import { EventDialogView } from './event-dialog';
import { emptyEvent } from './event-draft';

const draft = emptyEvent('2026-03-14', DEFAULT_EVENT_COLOUR);

describe('EventDialogView', () => {
  it('needs a title and a valid range before it can save', () => {
    const empty = renderWithProviders(
      <EventDialogView open onOpenChange={() => {}} value={draft} onChange={() => {}} onSubmit={() => {}} />,
    );
    expect(empty.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
    empty.unmount();

    const bad = renderWithProviders(
      <EventDialogView
        open
        onOpenChange={() => {}}
        value={{ ...draft, title: 'Bakım', allDay: false, startTime: '10:00', endTime: '09:00' }}
        onChange={() => {}}
        onSubmit={() => {}}
      />,
    );
    expect(bad.getByText('Bitiş başlangıçtan sonra olmalı')).toBeInTheDocument();
    expect(bad.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
  });

  it('hides the times for an all-day event and offers the colour palette', () => {
    const r = renderWithProviders(
      <EventDialogView open onOpenChange={() => {}} value={{ ...draft, title: 'Bakım' }} onChange={() => {}} onSubmit={() => {}} />,
    );
    expect(r.queryByLabelText('Başlangıç saati')).toBeNull();
    expect(r.getByRole('radio', { name: 'Camgöbeği' })).toBeInTheDocument();
    expect(r.getByRole('button', { name: 'Kaydet' })).toBeEnabled();
  });

  it('offers deletion only for an existing event, and nothing at all read-only', () => {
    const onDelete = vi.fn();
    const existing = renderWithProviders(
      <EventDialogView
        open
        onOpenChange={() => {}}
        value={{ ...draft, id: 'e-1', title: 'Bakım' }}
        onChange={() => {}}
        onSubmit={() => {}}
        onDelete={onDelete}
      />,
    );
    expect(existing.getByRole('button', { name: 'Etkinliği sil' })).toBeInTheDocument();
    existing.unmount();

    const readOnly = renderWithProviders(
      <EventDialogView
        open
        onOpenChange={() => {}}
        value={{ ...draft, id: 'e-1', title: 'Bakım' }}
        onChange={() => {}}
        onSubmit={() => {}}
        readOnly
      />,
    );
    expect(readOnly.queryByRole('button', { name: 'Kaydet' })).toBeNull();
    expect(readOnly.getByLabelText(/Başlık/)).toBeDisabled();
  });
});
