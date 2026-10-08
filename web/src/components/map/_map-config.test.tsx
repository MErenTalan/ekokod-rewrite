import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { MapConfigProvider, useMapTileUrl } from './_map-config';

function Probe() {
  return <output>{useMapTileUrl() || '(none)'}</output>;
}

describe('MapConfigProvider', () => {
  it('hands the server-read tile source to the maps below it, and none without a provider', () => {
    expect(render(<MapConfigProvider tileUrl="https://tiles.example.com/style.json"><Probe /></MapConfigProvider>).container).toHaveTextContent('https://tiles.example.com/style.json');
    expect(render(<Probe />).container).toHaveTextContent('(none)');
  });
});
