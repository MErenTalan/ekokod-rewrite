import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Map, type MapMarker } from './map';

const markers: MapMarker[] = [
  { id: 'ist', name: 'İstanbul Merkez Bina', lat: 41.015137, lng: 28.97953, status: 'active' },
  { id: 'ank', name: 'Ankara Soğuk Hava Deposu', lat: 39.92077, lng: 32.85411, status: 'active' },
  { id: 'izm', name: 'İzmir Eski Depo', lat: 38.423733, lng: 27.142826, status: 'passive' },
  { id: 'gap', name: 'Şanlıurfa GES Sahası', lat: 37.159149, lng: 38.796191, status: 'active' },
];

const meta = { title: 'Map/Map', component: Map, args: { markers, label: 'Bina ve santral konumları', onMarkerSelect: () => {} } } satisfies Meta<typeof Map>;
export default meta;
type Story = StoryObj<typeof meta>;

// The only map story the sweep runs: offline, no tile source (plan D23).
export const Fallback: Story = { args: { tileUrl: '' } };
// Needs network tiles, so it is excluded from the a11y sweep.
export const WithTiles: Story = { tags: ['no-sweep'], args: { tileUrl: 'https://demotiles.maplibre.org/style.json' } };
