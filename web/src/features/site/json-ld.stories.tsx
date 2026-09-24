import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { JsonLd } from './json-ld';

/** Invisible by design: the story documents the component and keeps the gallery complete. */
const meta = { title: 'Features/Site/JsonLd', component: JsonLd, tags: ['no-sweep'], args: { data: { '@type': 'Organization', name: 'EkoKod' } } } satisfies Meta<typeof JsonLd>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Organization: Story = {};
