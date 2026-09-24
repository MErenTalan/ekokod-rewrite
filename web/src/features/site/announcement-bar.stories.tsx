import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { AnnouncementBar } from './announcement-bar';

const meta = { title: 'Features/Site/AnnouncementBar', component: AnnouncementBar, parameters: { layout: 'fullscreen' } } satisfies Meta<typeof AnnouncementBar>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
