import type { Translator } from '@/lib/api/types';

/** The ten profiles the API serves, in display order. */
export const PROFILE_KEYS = [
  'weekday',
  'weekend',
  'winter_weekday',
  'winter_weekend',
  'spring_weekday',
  'spring_weekend',
  'summer_weekday',
  'summer_weekend',
  'autumn_weekday',
  'autumn_weekend',
] as const;
export type ProfileKey = (typeof PROFILE_KEYS)[number];

export const SEASONAL_KEYS = PROFILE_KEYS.filter((key) => key.includes('_')) as readonly ProfileKey[];

/**
 * Labels are derived from the key, never from a hand-written map: legacy
 * mislabelled three of the eight seasonal profiles (10 item 14), which a map
 * makes possible and this makes structurally impossible.
 */
export function profileLabel(t: Translator, key: ProfileKey): string {
  const [first, second] = key.split('_');
  if (!second) return t(`dayType.${first}`);
  return `${t(`season.${first}`)} – ${t(`dayType.${second}`)}`;
}

/** The explanatory note under each chart (01 §7.4). */
export function profileNote(t: Translator, key: ProfileKey): string {
  const [first, second] = key.split('_');
  return second ? t('note.seasonal', { season: t(`season.${first}`), dayType: t(`dayType.${second}`) }) : t(`note.${first}`);
}
