import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoAlarms, demoEvents } from './_fixture';
import { EventsDialogView } from './events-dialog';

const view = (events = demoEvents) =>
  renderWithProviders(
    <EventsDialogView open alarm={demoAlarms[0]} events={events} onClose={() => {}} />,
  );

describe('EventsDialogView', () => {
  it('shows the rule’s settings above its history (§7.12 details + logs)', () => {
    const r = view();
    expect(r.getByText('Kural ayarları')).toBeVisible();
    expect(r.getByText('Reaktif Limit Algılama Alarmı')).toBeVisible();
    expect(r.getByText('A-1')).toBeVisible();
  });

  it('distinguishes delivered, failed and pending', () => {
    const pending = [{ ...demoEvents[0], id: 'ev-3', notified_at: null, notification_error: null }];
    const r = view([...demoEvents, ...pending] as typeof demoEvents);
    expect(r.getByText('Gönderildi')).toBeVisible();
    // "fired and nobody was told" is its own state, not a missing row.
    expect(r.getByText('Gönderilemedi')).toBeVisible();
    expect(r.getByText('Bekliyor')).toBeVisible();
  });

  it('shows why a notification failed', () => {
    const r = view();
    expect(r.getByText(/535 authentication failed/)).toBeVisible();
  });

  it('says the rule has not fired when there is no history', () => {
    const r = view([]);
    expect(r.getByText('Log kaydı bulunamadı.')).toBeVisible();
  });
});
