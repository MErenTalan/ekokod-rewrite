import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { HomeView } from './home-view';

const meta = { title: 'Features/Site/HomeView', component: HomeView, args: { pricing: false }, parameters: { layout: 'fullscreen' } } satisfies Meta<typeof HomeView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const WithPricing: Story = { args: { pricing: true } };
