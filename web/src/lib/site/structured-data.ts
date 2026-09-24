/** Serialises JSON-LD for a <script> element: `<` is escaped so the payload cannot close it. */
export function jsonLd(data: unknown): string {
  return JSON.stringify(data).replace(/</g, '\\u003c');
}

export function organizationLd(base: string) {
  return {
    '@context': 'https://schema.org',
    '@type': 'Organization',
    name: 'EkoKod',
    url: base,
    email: 'info@ekokod.com',
    telephone: '+90 506 315 41 98',
    address: {
      '@type': 'PostalAddress',
      streetAddress: 'Akademi Mah. Gürbulut Sk. No: 67, Konya Teknokent',
      addressLocality: 'Selçuklu',
      addressRegion: 'Konya',
      addressCountry: 'TR',
    },
  };
}
