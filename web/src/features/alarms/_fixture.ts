import type { Alarm, AlarmEvaluation, AlarmEvent, Analyzer } from '@/lib/api/types';

/** Data-free stories still need shapes to render; these live here so no story
 *  file carries its own (R199). */
export const demoAnalyzers = [
  { id: 'a-1', installation_number: 'A-1', meter_multiplier: '1', is_active: true, provider: 'osos', provider_subtype: 'Baskent' },
  { id: 'a-2', installation_number: 'A-2', meter_multiplier: '1', is_active: true, provider: 'gridbox', provider_subtype: 'default' },
] as Analyzer[];

export const demoAlarms = [
  {
    id: 'al-1', name: 'Endüktif izleme', type: 'reactive_limit', is_enabled: true,
    analyzers: [{ id: 'a-1', installation_number: 'A-1', building_id: 'b-1' }],
    channels: [{ channel: 'email', target: 'ops@ornek.com.tr' }],
    notification_frequency_value: 6, notification_frequency_unit: 'hours',
    settings: { inductive_ratio_threshold: '20', inductive_period_value: 24, inductive_period_unit: 'hours' },
    created_at: '2026-09-18T12:00:00+03:00', updated_at: '2026-09-18T12:00:00+03:00',
  },
  {
    id: 'al-2', name: 'İletişim kopukluğu', type: 'data_communication', is_enabled: false,
    analyzers: [{ id: 'a-2', installation_number: 'A-2', building_id: 'b-1' }],
    channels: [{ channel: 'email', target: 'ops@ornek.com.tr' }, { channel: 'sms', target: '+905551112233' }],
    notification_frequency_value: null, notification_frequency_unit: null,
    settings: { communication_threshold_hours: 6 },
    created_at: '2026-09-18T12:00:00+03:00', updated_at: '2026-09-18T12:00:00+03:00',
  },
] as Alarm[];

export const demoEvents = [
  {
    id: 'ev-1', analyzer_id: 'a-1', triggered_at: '2026-09-18T11:00:00+03:00',
    message: 'Endüktif izleme — A-1',
    detail: { lines: ['Endüktif oran %25 eşiği aştı (%20)'] },
    notified_at: '2026-09-18T11:00:30+03:00', notification_error: null,
  },
  {
    id: 'ev-2', analyzer_id: 'a-1', triggered_at: '2026-09-17T11:00:00+03:00',
    message: 'Endüktif izleme — A-1', detail: null,
    notified_at: null, notification_error: 'e-posta gönderilemedi: 535 authentication failed',
  },
] as unknown as AlarmEvent[];

export const demoEvaluation = {
  evaluated_at: '2026-09-18T12:00:00+03:00',
  dry_run: true,
  notifications_sent: 0,
  analyzers: [
    {
      analyzer_id: 'a-1', installation_number: 'A-1', fired: true, no_verdict: [],
      breaches: [{ field: 'inductive_ratio_threshold', measured: '25', threshold: '20', message: 'Endüktif oran %25 eşiği aştı (%20)' }],
    },
    { analyzer_id: 'a-2', installation_number: 'A-2', fired: false, no_verdict: ['communication_threshold_hours'], breaches: [] },
  ],
} as AlarmEvaluation;
