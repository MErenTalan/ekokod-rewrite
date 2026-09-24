import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { PricingView } from './pricing-view';

const meta = { title: 'Features/Site/PricingView', component: PricingView, parameters: { layout: 'fullscreen' } } satisfies Meta<typeof PricingView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
