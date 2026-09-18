import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoAnalyzers } from './_fixture';
import { AlarmDialogView } from './alarm-dialog';
import { emptyDraft, type AlarmDraft } from './alarm-draft';

const draft = (over: Partial<AlarmDraft> = {}): AlarmDraft => ({
  ...emptyDraft(), name: 'Kural', analyzerIds: ['a-1'], ...over,
});

const view = (over: Partial<AlarmDraft> = {}, onDraftChange = vi.fn()) =>
  renderWithProviders(
    <AlarmDialogView
      open
      draft={draft(over)}
      analyzers={demoAnalyzers}
      onDraftChange={onDraftChange}
      onSubmit={() => {}}
      onClose={() => {}}
    />,
  );

describe('AlarmDialogView', () => {
  it('shows only the reactive fields for a reactive rule', () => {
    const r = view({ type: 'reactive_limit' });
    expect(r.getByLabelText(/Endüktif Oran/)).toBeVisible();
    expect(r.queryByLabelText(/İletişim Kesinti/)).toBeNull();
  });

  it('shows only the comms threshold for a comms rule', () => {
    const r = view({ type: 'data_communication' });
    expect(r.getByLabelText(/İletişim Kesinti/)).toBeVisible();
    expect(r.queryByLabelText(/Endüktif Oran/)).toBeNull();
  });

  it('offers power bounds and no voltage at all (R212)', () => {
    const r = view({ type: 'current_voltage_power' });
    expect(r.getByLabelText(/Güç Maks/)).toBeVisible();
    expect(r.getByLabelText(/Güç Min/)).toBeVisible();
    expect(r.queryByLabelText(/Gerilim/)).toBeNull();
    expect(r.queryByLabelText(/Voltaj/)).toBeNull();
  });

  it('explains what the invoice threshold compares', () => {
    const r = view({ type: 'invoice_increase' });
    expect(r.getByText(/Son fatura bir önceki faturadan/)).toBeVisible();
  });

  it('says SMS is not sent yet, and only when the channel is ticked (R211)', () => {
    expect(view({ channels: { email: false, sms: false } })
      .queryByText(/SMS bildirimi henüz etkin değil/)).toBeNull();
    const r = view({ channels: { email: false, sms: true } });
    expect(r.getByText(/SMS bildirimi henüz etkin değil/)).toBeVisible();
    expect(r.getByLabelText(/SMS Numaraları/)).toBeVisible();
  });

  it('refuses to save while a required limit is missing (R230)', () => {
    const r = view({ type: 'data_communication', settings: {} });
    expect(r.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
  });

  it('enables saving once the rule is complete', () => {
    const r = view({ type: 'data_communication', settings: { communication_threshold_hours: '6' } });
    expect(r.getByRole('button', { name: 'Kaydet' })).toBeEnabled();
  });
});
