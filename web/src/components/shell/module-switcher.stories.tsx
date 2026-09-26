import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { ModuleSwitcher } from './module-switcher';

const meta = { title: 'Shell/ModuleSwitcher', component: ModuleSwitcher, args: { current: 'ekorm', modules: ['ekorm', 'ekocm'] } } satisfies Meta<typeof ModuleSwitcher>;
export default meta;
type Story = StoryObj<typeof meta>;

export const InEkorm: Story = {};
export const InEkocm: Story = { args: { current: 'ekocm' } };
