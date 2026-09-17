import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { LanguageSwitcher } from './language-switcher';

const meta = { title: 'Shell/LanguageSwitcher', component: LanguageSwitcher } satisfies Meta<typeof LanguageSwitcher>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
