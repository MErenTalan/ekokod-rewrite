// Calendar event colours are DATA: the API stores them as `#rrggbb` (R203), so
// this is the one place outside tokens.css that may name colour literals (D5).
// They are drawn as a border and a dot beside text that keeps its own token
// colour, so no status ever rests on hue alone (07 §2.4).
export const EVENT_COLOURS = [
  { id: 'emerald', hex: '#059669' },
  { id: 'cyan', hex: '#0e7490' },
  { id: 'amber', hex: '#a16207' },
  { id: 'violet', hex: '#7e22ce' },
  { id: 'red', hex: '#b91c1c' },
  { id: 'slate', hex: '#64748b' },
] as const;

export type EventColourId = (typeof EVENT_COLOURS)[number]['id'];
export const DEFAULT_EVENT_COLOUR = EVENT_COLOURS[0].hex;
