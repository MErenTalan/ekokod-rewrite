import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoArchive } from './_fixture';
import { ArchiveTabView } from './archive-tab';

const meta = {
  title: 'Features/Reports/ArchiveTab',
  component: ArchiveTabView,
  args: {
    items: demoArchive, total: demoArchive.length, loading: false,
    filters: { type: 'all', buildingId: 'all' },
    buildings: [{ value: 'b-1', label: 'Merkez' }, { value: 'b-2', label: 'Depo' }],
    onFilters: () => {}, onDownload: () => {},
  },
} satisfies Meta<typeof ArchiveTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { items: [], total: 0 } };
export const Loading: Story = { args: { loading: true } };
