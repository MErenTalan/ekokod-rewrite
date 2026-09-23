import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoNationalTariffs } from './_fixture';
import { DefaultsTabView } from './defaults-tab';

const meta = {
  title: 'Features/Tariffs/DefaultsTab',
  component: DefaultsTabView,
  args: { entries: demoNationalTariffs, onPublish: () => {}, onDelete: () => {} },
} satisfies Meta<typeof DefaultsTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { entries: [] } };
