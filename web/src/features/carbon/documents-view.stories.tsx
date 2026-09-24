import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { DocumentsView } from './documents-view';

const meta = { title: 'Features/Carbon/Documents', component: DocumentsView } satisfies Meta<typeof DocumentsView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Unavailable: Story = {};
