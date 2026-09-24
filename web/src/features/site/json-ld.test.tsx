import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { JsonLd } from './json-ld';

describe('JsonLd', () => {
  it('renders one ld+json script that round-trips', () => {
    const { container } = renderWithProviders(<JsonLd data={{ '@type': 'Thing', name: '</script>x' }} />);
    const scripts = container.querySelectorAll('script[type="application/ld+json"]');
    expect(scripts).toHaveLength(1);
    expect(JSON.parse(scripts[0].textContent ?? '')).toEqual({ '@type': 'Thing', name: '</script>x' });
  });
});
