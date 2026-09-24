import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { DetailsTabView } from './details-tab';
import { demoStatistics } from './_fixture';

const meta = {
  title: 'Features/LoadProfile/DetailsTab',
  component: DetailsTabView,
  args: { statistics: demoStatistics, onExport: () => {} },
} satisfies Meta<typeof DetailsTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { statistics: null } };
