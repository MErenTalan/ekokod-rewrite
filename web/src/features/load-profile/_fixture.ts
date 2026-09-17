import type { LoadProfiles, LoadProfileStatistics } from '@/lib/api/types';

import { PROFILE_KEYS } from './profile-label';

/** A deterministic 24-hour curve: a working-day shape with a midday peak. */
const curve = (base: number) =>
  Array.from({ length: 24 }, (_, hour) => String(base + Math.round(base * 0.8 * Math.sin((hour / 24) * Math.PI))));

export const demoProfiles: LoadProfiles = {
  profiles: Object.fromEntries(PROFILE_KEYS.map((key, i) => [key, curve(20 + i * 3)])),
  days: Object.fromEntries(PROFILE_KEYS.map((key, i) => [key, 10 + i])),
  config: { weekend_days: [0, 6], weekend_source: 'company', vacations: 1 },
} as unknown as LoadProfiles;

export const demoStatistics: LoadProfileStatistics = {
  config: demoProfiles.config,
  statistics: Object.fromEntries(
    PROFILE_KEYS.map((key, i) => [
      key,
      {
        max: String(40 + i),
        min: String(8 + i),
        hour_of_max: 13,
        mean: String(24 + i),
        stddev: '7.5',
        range: String(32),
        load_factor: '0.58',
      },
    ]),
  ),
} as unknown as LoadProfileStatistics;
