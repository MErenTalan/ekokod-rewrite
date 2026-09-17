import { describe, expect, it } from 'vitest';

import { messages } from '../../../messages';

import { PROFILE_KEYS, profileLabel, type ProfileKey } from './profile-label';

const translator = (locale: 'tr' | 'en') => (key: string, values?: Record<string, string | number>) => {
  const raw = key.split('.').reduce<unknown>((node, part) => (node as Record<string, unknown>)[part], messages[locale].loadProfile);
  return String(raw).replace(/\{(\w+)\}/g, (_, name: string) => String(values?.[name] ?? ''));
};

// 09 §F6 acceptance: all eight seasonal labels are correct — three were wrong
// in the legacy product (10 item 14), so they are asserted literally here.
describe('profileLabel', () => {
  it('names every profile in Turkish', () => {
    const t = translator('tr');
    expect(PROFILE_KEYS.map((key) => profileLabel(t, key))).toEqual([
      'Hafta içi',
      'Hafta sonu',
      'Kış – Hafta içi',
      'Kış – Hafta sonu',
      'İlkbahar – Hafta içi',
      'İlkbahar – Hafta sonu',
      'Yaz – Hafta içi',
      'Yaz – Hafta sonu',
      'Sonbahar – Hafta içi',
      'Sonbahar – Hafta sonu',
    ]);
  });

  it('names every profile in English', () => {
    const t = translator('en');
    expect(PROFILE_KEYS.map((key) => profileLabel(t, key))).toEqual([
      'Weekday',
      'Weekend',
      'Winter – Weekday',
      'Winter – Weekend',
      'Spring – Weekday',
      'Spring – Weekend',
      'Summer – Weekday',
      'Summer – Weekend',
      'Autumn – Weekday',
      'Autumn – Weekend',
    ]);
  });

  it('derives the label from the key, so a summer weekend can never read as a weekday', () => {
    const t = translator('tr');
    for (const key of PROFILE_KEYS) {
      const [first, second] = key.split('_');
      const label = profileLabel(t, key as ProfileKey);
      if (second) {
        expect(label).toContain(messages.tr.loadProfile.season[first as 'winter']);
        expect(label).toContain(messages.tr.loadProfile.dayType[second as 'weekday']);
      } else {
        expect(label).toBe(messages.tr.loadProfile.dayType[first as 'weekday']);
      }
    }
  });
});
