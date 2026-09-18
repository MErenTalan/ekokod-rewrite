import type { AlarmKind } from './alarm-draft';

/** The i18n keys for the four rule types, typed so next-intl can check them.
 *
 *  R212 renamed current_voltage_power to a power alarm: no provider reports
 *  voltage or current, so the legacy label promised two measurements the
 *  product never had. */
export const TYPE_LABEL_KEYS = {
  reactive_limit: 'types.reactiveLimit',
  data_communication: 'types.dataCommunication',
  current_voltage_power: 'types.power',
  invoice_increase: 'types.invoiceIncrease',
} as const satisfies Record<AlarmKind, string>;

export type TypeLabelKey = (typeof TYPE_LABEL_KEYS)[AlarmKind];

export function typeLabelKey(type: string): TypeLabelKey {
  return TYPE_LABEL_KEYS[type as AlarmKind] ?? TYPE_LABEL_KEYS.current_voltage_power;
}

/** The four types in the order the dialog offers them. */
export const ALARM_KINDS: AlarmKind[] = [
  'reactive_limit', 'data_communication', 'current_voltage_power', 'invoice_increase',
];
