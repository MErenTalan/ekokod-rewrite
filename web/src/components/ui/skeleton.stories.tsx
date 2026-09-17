import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Skeleton, SkeletonText } from './skeleton';

const meta = { title: 'UI/Skeleton', component: Skeleton } satisfies Meta<typeof Skeleton>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <div className="flex max-w-sm flex-col gap-3">
      <Skeleton className="h-8 w-40" />
      <SkeletonText lines={3} />
    </div>
  ),
};
