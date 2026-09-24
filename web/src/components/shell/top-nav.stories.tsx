import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { TopNav } from './top-nav';

const meta = { title: 'Shell/TopNav', component: TopNav, parameters: { layout: 'fullscreen', nextjs: { appDirectory: true, navigation: { pathname: '/ekorm/reports' } } } } satisfies Meta<typeof TopNav>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
