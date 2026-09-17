// Deterministic sample energy data for chart stories (no Math.random: screenshots and axe runs stay stable).
import type { Datum } from './_theme';

export const MONTHS = ['Oca', 'Şub', 'Mar', 'Nis', 'May', 'Haz', 'Tem', 'Ağu', 'Eyl', 'Eki', 'Kas', 'Ara'];
export const WEEKDAYS = ['Pzt', 'Sal', 'Çar', 'Per', 'Cum', 'Cmt', 'Paz'];
export const wave = (i: number, base: number, amp: number, period = 12, phase = 0) =>
  (base + amp * Math.sin(((i + phase) / period) * 2 * Math.PI)).toFixed(3);
export const empty = { title: 'Bu dönem için ölçüm yok', description: 'Analizörün veri gönderdiğini kontrol edin veya başka bir dönem seçin.' };
export const monthly = (keys: Record<string, [number, number, number?]>): Datum[] =>
  MONTHS.map((x, i) => ({ x, ...Object.fromEntries(Object.entries(keys).map(([k, [base, amp, phase]]) => [k, wave(i, base, amp, 12, phase ?? 0)])) }));
