import type { Alarm, AlarmAnalyzer, AlarmChannel, AlarmFields, AlarmSettings } from '@/lib/api/types';

/** The four rule types of 01 §7.12. */
export type AlarmKind = NonNullable<AlarmFields['type']>;

/** A settings field name, narrowed to the string keys the forms use. */
export type SettingsKey = Extract<keyof AlarmSettings, string>;

/**
 * Which settings each type owns — the mirror of the Go switch in
 * internal/domain/alarm and internal/service/alarms.
 *
 * voltage_max and voltage_min appear NOWHERE: no provider reports voltage, so
 * R212 removed the thresholds rather than leaving a field that never fires.
 */
export const SETTINGS_BY_TYPE: Record<AlarmKind, readonly SettingsKey[]> = {
  reactive_limit: [
    'inductive_ratio_threshold', 'inductive_period_value', 'inductive_period_unit',
    'capacitive_ratio_threshold', 'capacitive_period_value', 'capacitive_period_unit',
    'active_consumption_max', 'active_consumption_max_period_value', 'active_consumption_max_period_unit',
    'active_consumption_min', 'active_consumption_min_period_value', 'active_consumption_min_period_unit',
  ],
  data_communication: ['communication_threshold_hours'],
  current_voltage_power: ['power_max', 'power_min'],
  invoice_increase: ['invoice_threshold_pct'],
};

/** The four reactive groups; a group counts only with its threshold AND period (R230). */
export const REACTIVE_GROUPS = [
  ['inductive_ratio_threshold', 'inductive_period_value', 'inductive_period_unit'],
  ['capacitive_ratio_threshold', 'capacitive_period_value', 'capacitive_period_unit'],
  ['active_consumption_max', 'active_consumption_max_period_value', 'active_consumption_max_period_unit'],
  ['active_consumption_min', 'active_consumption_min_period_value', 'active_consumption_min_period_unit'],
] as const satisfies readonly (readonly SettingsKey[])[];

export type PeriodUnit = 'hours' | 'days';

export type AlarmDraft = {
  id?: string;
  name: string;
  type: AlarmKind;
  isEnabled: boolean;
  analyzerIds: string[];
  /** Comma-separated, as §7.12 specifies. */
  emailTargets: string;
  /** Stored so a gateway can be wired later; never sent (R211). */
  smsTargets: string;
  channels: { email: boolean; sms: boolean };
  frequencyValue: string;
  frequencyUnit: PeriodUnit | '';
  /** Raw field strings keyed by the DTO's own names. */
  settings: Partial<Record<SettingsKey, string>>;
};

export function emptyDraft(): AlarmDraft {
  return {
    name: '', type: 'reactive_limit', isEnabled: true, analyzerIds: [],
    emailTargets: '', smsTargets: '', channels: { email: false, sms: false },
    frequencyValue: '', frequencyUnit: 'hours', settings: {},
  };
}

/** Builds a draft from a stored rule, for the edit dialog. */
export function draftFrom(alarm: Alarm): AlarmDraft {
  const settings: AlarmDraft['settings'] = {};
  for (const key of SETTINGS_BY_TYPE[alarm.type as AlarmKind]) {
    const value = alarm.settings?.[key];
    if (value !== undefined && value !== null) settings[key] = String(value);
  }
  const targets = (channel: 'email' | 'sms') =>
    (alarm.channels ?? []).filter((c: AlarmChannel) => c.channel === channel).map((c: AlarmChannel) => c.target);
  const emails = targets('email');
  const numbers = targets('sms');
  return {
    id: alarm.id,
    name: alarm.name,
    type: alarm.type as AlarmKind,
    isEnabled: alarm.is_enabled,
    analyzerIds: (alarm.analyzers ?? []).map((a: AlarmAnalyzer) => a.id),
    emailTargets: emails.join(', '),
    smsTargets: numbers.join(', '),
    channels: { email: emails.length > 0, sms: numbers.length > 0 },
    frequencyValue: alarm.notification_frequency_value == null ? '' : String(alarm.notification_frequency_value),
    frequencyUnit: (alarm.notification_frequency_unit as PeriodUnit | null) ?? 'hours',
    settings,
  };
}

/**
 * Switches the type, keeping only the settings the new type owns (R229).
 *
 * Without this, a threshold typed under one type and abandoned under another
 * would ride along in the request — which the server would then have to strip.
 */
export function clearForType(draft: AlarmDraft, type: AlarmKind): AlarmDraft {
  const kept: AlarmDraft['settings'] = {};
  for (const key of SETTINGS_BY_TYPE[type]) {
    if (draft.settings[key] !== undefined) kept[key] = draft.settings[key];
  }
  return { ...draft, type, settings: kept };
}

const splitTargets = (raw: string) =>
  raw.split(',').map((t) => t.trim()).filter((t) => t.length > 0);

const EMAIL_RE = /^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$/;
const PHONE_RE = /^\+?[0-9]{10,15}$/;

/** The 422 the server would return, checked here first (R230, R231). */
export function draftErrors(draft: AlarmDraft): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!draft.name.trim()) errors.name = 'required';
  if (draft.analyzerIds.length === 0) errors.analyzerIds = 'required';

  const has = (key: SettingsKey) => Boolean(draft.settings[key]?.trim());
  switch (draft.type) {
    case 'reactive_limit':
      if (!REACTIVE_GROUPS.some((group) => group.every(has))) errors.settings = 'atLeastOne';
      break;
    case 'data_communication':
      if (!has('communication_threshold_hours')) errors.communication_threshold_hours = 'required';
      break;
    case 'current_voltage_power':
      if (!has('power_max') && !has('power_min')) errors.settings = 'atLeastOne';
      break;
    case 'invoice_increase':
      if (!has('invoice_threshold_pct')) errors.invoice_threshold_pct = 'required';
      break;
  }

  if (draft.frequencyValue.trim() && !draft.frequencyUnit) errors.frequencyUnit = 'required';
  if (draft.channels.email) {
    const emails = splitTargets(draft.emailTargets);
    if (emails.length === 0) errors.emailTargets = 'required';
    else if (!emails.every((e) => EMAIL_RE.test(e))) errors.emailTargets = 'invalid';
  }
  if (draft.channels.sms) {
    const numbers = splitTargets(draft.smsTargets);
    if (numbers.length === 0) errors.smsTargets = 'required';
    else if (!numbers.every((n) => PHONE_RE.test(n))) errors.smsTargets = 'invalid';
  }
  return errors;
}

/**
 * Maps a draft to the request body. Only the type's own settings are emitted,
 * so a voltage threshold can never be sent even if one were in the draft.
 */
export function toAlarmRequest(draft: AlarmDraft): AlarmFields {
  const settings: AlarmSettings = {};
  for (const key of SETTINGS_BY_TYPE[draft.type]) {
    const raw = draft.settings[key]?.trim();
    if (!raw) continue;
    // Period values and the hour threshold are integers; every other number
    // crosses as a string, because 05 §1 forbids JSON numbers for decimals.
    settings[key] = (key.endsWith('_period_value') || key === 'communication_threshold_hours'
      ? Number(raw)
      : raw) as never;
  }

  const channels: NonNullable<AlarmFields['channels']> = [];
  if (draft.channels.email) {
    for (const target of splitTargets(draft.emailTargets)) channels.push({ channel: 'email', target });
  }
  if (draft.channels.sms) {
    for (const target of splitTargets(draft.smsTargets)) channels.push({ channel: 'sms', target });
  }

  const frequency = draft.frequencyValue.trim();
  return {
    name: draft.name.trim(),
    type: draft.type,
    is_enabled: draft.isEnabled,
    analyzer_ids: draft.analyzerIds,
    channels,
    ...(frequency
      ? { notification_frequency_value: Number(frequency), notification_frequency_unit: draft.frequencyUnit || 'hours' }
      : {}),
    settings,
  };
}
