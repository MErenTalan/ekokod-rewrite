import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { AboutView } from './about-view';

const meta = { title: 'Features/Site/AboutView', component: AboutView, parameters: { layout: 'fullscreen' } } satisfies Meta<typeof AboutView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
