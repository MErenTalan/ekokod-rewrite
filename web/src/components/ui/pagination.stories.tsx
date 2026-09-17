import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useState } from 'react';

import { Pagination } from './pagination';

function Stateful({ pageCount = 12 }: { pageCount?: number }) {
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(25);
  return <Pagination page={page} pageCount={pageCount} onPageChange={setPage} pageSize={size} pageSizeOptions={[25, 50, 100]} onPageSizeChange={setSize} totalItems={2873} />;
}

const meta = { title: 'UI/Pagination', component: Pagination, args: { page: 1, pageCount: 1, onPageChange: () => {} } } satisfies Meta<typeof Pagination>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Stateful /> };
export const SinglePage: Story = { args: { page: 1, pageCount: 1 } };
