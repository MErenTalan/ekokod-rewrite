import type { components } from '@/lib/api/schema';

export type CalcGroup = components['schemas']['PublicBillRequest']['user_group'];
export type CalcField = 'group' | 'voltage' | 'term' | 'multi' | 'start' | 'end' | 'total' | 't1' | 't2' | 't3' | 'demand' | 'contract';
export type CalcValues = {
  group: CalcGroup;
  voltage: 'lv' | 'mv';
  term: 'monomial' | 'binomial';
  multi: boolean;
  start: string | null;
  end: string | null;
  total: string | null;
  t1: string | null;
  t2: string | null;
  t3: string | null;
  demand: string | null;
  contract: string | null;
};

const iso = (y: number, m: number, d: number) => `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`;

/** Last calendar month relative to `today` (YYYY-MM-DD, already in Istanbul time). */
export function defaultPeriod(today: string): { start: string; end: string } {
  const [y, m] = today.split('-').map(Number);
  const [py, pm] = m === 1 ? [y - 1, 12] : [y, m - 1];
  const last = new Date(Date.UTC(py, pm, 0)).getUTCDate();
  return { start: iso(py, pm, 1), end: iso(py, pm, last) };
}

export function initialValues(today: string): CalcValues {
  const { start, end } = defaultPeriod(today);
  return { group: 'residential', voltage: 'lv', term: 'monomial', multi: false, start, end, total: null, t1: null, t2: null, t3: null, demand: null, contract: null };
}

/** R351: double term exists only at MV, and lighting has no T prices. */
export function withChange(v: CalcValues, patch: Partial<CalcValues>): CalcValues {
  const next = { ...v, ...patch };
  if (next.voltage === 'lv') next.term = 'monomial';
  if (next.group === 'lighting') next.multi = false;
  return next;
}

export function calculatorBody(v: CalcValues): components['schemas']['PublicBillRequest'] {
  const opt = (key: string, value: string | null) => (value ? { [key]: value } : {});
  return {
    user_group: v.group,
    voltage_level: v.voltage,
    term: v.term,
    multi_time: v.multi,
    start: v.start ?? '',
    end: v.end ?? '',
    ...opt('total_consumption', v.total),
    ...opt('t1', v.t1),
    ...opt('t2', v.t2),
    ...opt('t3', v.t3),
    ...(v.term === 'binomial' ? { ...opt('demand', v.demand), ...opt('contract_power', v.contract) } : {}),
  };
}

/** The API's field names (DTO and domain refusals) mapped onto the form's. */
export const API_FIELDS: Record<string, CalcField> = {
  user_group: 'group', group: 'group', voltage_level: 'voltage', term: 'term', multi_time: 'multi', start: 'start', end: 'end',
  total_consumption: 'total', t1: 't1', t2: 't2', t3: 't3', demand: 'demand', contract_power: 'contract',
};
