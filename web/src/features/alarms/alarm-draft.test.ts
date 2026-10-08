import { describe, expect, it } from 'vitest';

import { clearForType, draftErrors, draftFrom, emptyDraft, serverFieldErrors, toAlarmRequest } from './alarm-draft';

const base = { ...emptyDraft(), name: 'Test', analyzerIds: ['a-1'] };

describe('alarm draft', () => {
  it('clears the other types’ settings when the type switches (R229)', () => {
    const reactive = { ...base, type: 'reactive_limit' as const,
      settings: { inductive_ratio_threshold: '20', inductive_period_value: '24' } };
    const power = clearForType(reactive, 'current_voltage_power');
    expect(power.settings.inductive_ratio_threshold).toBeUndefined();
    expect(power.type).toBe('current_voltage_power');
  });

  it('keeps the settings the new type does own', () => {
    const comms = { ...base, type: 'data_communication' as const,
      settings: { communication_threshold_hours: '6' } };
    expect(clearForType(comms, 'data_communication').settings.communication_threshold_hours).toBe('6');
  });

  it('never sends a voltage threshold (R212)', () => {
    const power = { ...base, type: 'current_voltage_power' as const,
      settings: { power_max: '100', voltage_max: '400' } };
    const body = toAlarmRequest(power);
    expect(body.settings).not.toHaveProperty('voltage_max');
    expect(body.settings).toHaveProperty('power_max', '100');
  });

  it('requires a name, an analyzer and one limit (R230)', () => {
    expect(draftErrors(emptyDraft())).toHaveProperty('name');
    expect(draftErrors(emptyDraft())).toHaveProperty('analyzerIds');
    // A reactive threshold without its period is not a complete group.
    expect(draftErrors({ ...base, settings: { inductive_ratio_threshold: '20' } })).toHaveProperty('settings');
    expect(draftErrors({ ...base,
      settings: { inductive_ratio_threshold: '20', inductive_period_value: '24', inductive_period_unit: 'hours' },
    })).toEqual({});
  });

  it('counts a limit with its period even when the unit was left at the shown default', () => {
    const draft = { ...base, settings: { capacitive_ratio_threshold: '15', capacitive_period_value: '24' } };
    expect(draftErrors(draft)).toEqual({});
    expect(toAlarmRequest(draft).settings).toEqual({
      capacitive_ratio_threshold: '15', capacitive_period_value: 24, capacitive_period_unit: 'hours',
    });
  });

  it('names the missing half of a limit instead of asking for a limit', () => {
    expect(draftErrors({ ...base, settings: { capacitive_ratio_threshold: '15' } }).settings).toBe('periodMissing');
    expect(draftErrors({ ...base, settings: { capacitive_period_value: '24' } }).settings).toBe('periodMissing');
    expect(draftErrors({ ...base, settings: {} }).settings).toBe('atLeastOne');
    // A unit alone is what the select shows by default, not something typed.
    expect(draftErrors({ ...base, settings: { capacitive_period_unit: 'days' } }).settings).toBe('atLeastOne');
  });

  it('requires one power bound and one comms threshold', () => {
    expect(draftErrors({ ...base, type: 'current_voltage_power' })).toHaveProperty('settings');
    expect(draftErrors({ ...base, type: 'current_voltage_power', settings: { power_min: '5' } })).toEqual({});
    expect(draftErrors({ ...base, type: 'data_communication' })).toHaveProperty('communication_threshold_hours');
  });

  it('splits comma-separated recipients and drops blanks', () => {
    const draft = { ...base, type: 'data_communication' as const,
      settings: { communication_threshold_hours: '6' },
      channels: { email: true, sms: true },
      emailTargets: 'a@b.com, , c@d.com ', smsTargets: '+905551112233,' };
    expect(toAlarmRequest(draft).channels).toEqual([
      { channel: 'email', target: 'a@b.com' },
      { channel: 'email', target: 'c@d.com' },
      { channel: 'sms', target: '+905551112233' },
    ]);
  });

  it('omits a channel’s targets when the channel is unticked', () => {
    const draft = { ...base, type: 'data_communication' as const,
      settings: { communication_threshold_hours: '6' },
      channels: { email: true, sms: false }, emailTargets: 'a@b.com', smsTargets: '+905551112233' };
    expect(toAlarmRequest(draft).channels).toEqual([{ channel: 'email', target: 'a@b.com' }]);
  });

  it('rejects a malformed address or number before the server does (R231)', () => {
    const withEmail = { ...base, type: 'data_communication' as const,
      settings: { communication_threshold_hours: '6' },
      channels: { email: true, sms: false }, emailTargets: 'not-an-email' };
    expect(draftErrors(withEmail)).toHaveProperty('emailTargets', 'invalid');

    const withSMS = { ...base, type: 'data_communication' as const,
      settings: { communication_threshold_hours: '6' },
      channels: { email: false, sms: true }, smsTargets: '0555 123 45 67' };
    expect(draftErrors(withSMS)).toHaveProperty('smsTargets', 'invalid');
  });

  it('sends period values as integers and thresholds as strings (05 §1)', () => {
    const draft = { ...base, type: 'reactive_limit' as const,
      settings: { inductive_ratio_threshold: '20.5', inductive_period_value: '24', inductive_period_unit: 'hours' } };
    const settings = toAlarmRequest(draft).settings!;
    expect(settings.inductive_ratio_threshold).toBe('20.5');
    expect(settings.inductive_period_value).toBe(24);
  });

  it('round-trips a stored rule back into a draft', () => {
    const draft = draftFrom({
      id: 'al-1', name: 'İletişim', type: 'data_communication', is_enabled: false,
      analyzers: [{ id: 'a-1', installation_number: 'A-1', building_id: null }],
      channels: [{ channel: 'email', target: 'ops@example.com' }, { channel: 'sms', target: '+905551112233' }],
      notification_frequency_value: 6, notification_frequency_unit: 'hours',
      settings: { communication_threshold_hours: 6 },
      created_at: '2026-09-18T12:00:00+03:00', updated_at: '2026-09-18T12:00:00+03:00',
    } as never);
    expect(draft.name).toBe('İletişim');
    expect(draft.isEnabled).toBe(false);
    expect(draft.analyzerIds).toEqual(['a-1']);
    expect(draft.emailTargets).toBe('ops@example.com');
    expect(draft.smsTargets).toBe('+905551112233');
    expect(draft.channels).toEqual({ email: true, sms: true });
    expect(draft.settings.communication_threshold_hours).toBe('6');
    expect(draftErrors(draft)).toEqual({});
  });

  it('sends a Turkish-typed threshold with a point and marks a non-number', () => {
    const power = { ...base, type: 'current_voltage_power' as const, settings: { power_max: '1.250,5', power_min: '12,5' } };
    expect(toAlarmRequest(power).settings).toEqual({ power_max: '1250.5', power_min: '12.5' });
    expect(draftErrors({ ...power, settings: { power_max: '12,5,0' } }).power_max).toBe('invalid');
    const comms = { ...base, type: 'data_communication' as const, settings: { communication_threshold_hours: '6,5' } };
    expect(draftErrors(comms).communication_threshold_hours).toBe('invalid');
    expect(draftErrors({ ...base, frequencyValue: 'iki', frequencyUnit: 'hours' as const }).frequencyValue).toBe('invalid');
  });

  it('reads the API field errors by the dialog field they belong to', () => {
    const d = { ...base, channels: { email: true, sms: true }, emailTargets: 'a@b.co, c@d.co', smsTargets: '05321234567' };
    expect(
      serverFieldErrors(d, {
        'settings.power_max': 'x', analyzer_ids: 'y', 'channels[1].target': 'e', 'channels[2].target': 's',
        notification_frequency_value: 'f',
      }),
    ).toEqual({ power_max: 'x', analyzerIds: 'y', emailTargets: 'e', smsTargets: 's', frequencyValue: 'f' });
  });
});
