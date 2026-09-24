// The site's genuine, attributable content (Q-H10): legacy references, news and documents.

export const REFERENCES = [
  { key: 'ministry', logo: '/site/partners/ministry.webp', width: 339, height: 100 },
  { key: 'hacettepe', logo: '/site/partners/hacettepe.webp', width: 400, height: 589 },
  { key: 'arti', logo: '/site/partners/arti.webp', width: 400, height: 400 },
  { key: 'yalovares', logo: '/site/partners/yalovares.webp', width: 400, height: 84 },
] as const;
export type ReferenceKey = (typeof REFERENCES)[number]['key'];

export type Photo = { src: string; width: number; height: number };
export const NEWS: { key: 'forum' | 'stand' | 'mayors'; photos: Photo[]; video?: string }[] = [
  {
    key: 'forum',
    video: 'https://www.youtube.com/live/_ZqMbOPMjdg?t=363',
    photos: [
      { src: '/site/news/news-1.webp', width: 1200, height: 900 },
      { src: '/site/news/news-2.webp', width: 1200, height: 554 },
    ],
  },
  {
    key: 'stand',
    photos: [
      { src: '/site/news/news-3.webp', width: 1200, height: 1600 },
      { src: '/site/news/news-4.webp', width: 577, height: 433 },
      { src: '/site/news/news-5.webp', width: 325, height: 433 },
      { src: '/site/news/news-6.webp', width: 325, height: 433 },
    ],
  },
  { key: 'mayors', photos: [{ src: '/site/news/news-7.webp', width: 1200, height: 900 }] },
];

/** No downloadable files exist yet: each document is requested through the contact form (legacy "contact to download"). */
export const DOCUMENT_GROUPS = {
  general: ['userGuide', 'articles', 'needsForm'],
  technical: ['specification', 'api'],
  training: ['videos', 'quickStart'],
} as const;
export type DocumentKey = (typeof DOCUMENT_GROUPS)[keyof typeof DOCUMENT_GROUPS][number];

export const RM_FEATURES = ['realTime', 'carbon', 'anomalies', 'predict', 'penalty', 'bills', 'profiles'] as const;
export const CM_FEATURES = ['scope', 'iso', 'isoghg', 'emissionFactor', 'corporate', 'compare'] as const;
