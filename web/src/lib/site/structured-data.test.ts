import { describe, expect, it } from 'vitest';

import { jsonLd, organizationLd } from './structured-data';

describe('structured data', () => {
  it('describes the organisation with its contact point', () => {
    const org = organizationLd('https://ekokod.com');
    expect(org).toMatchObject({ '@context': 'https://schema.org', '@type': 'Organization', url: 'https://ekokod.com', email: 'info@ekokod.com' });
  });

  it('cannot close its script element', () => {
    expect(jsonLd({ name: '</script><script>alert(1)</script>' })).not.toContain('</script>');
    expect(JSON.parse(jsonLd({ name: '</script>' }))).toEqual({ name: '</script>' });
  });
});
