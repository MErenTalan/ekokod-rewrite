import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { DocumentsView, ReferencesView, ToolkitView } from './catalogue-views';

const meta = { title: 'Features/Site/CatalogueViews', component: ReferencesView, parameters: { layout: 'fullscreen' } } satisfies Meta<typeof ReferencesView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const References: Story = {};
export const Documents: Story = { render: () => <DocumentsView /> };
export const Toolkit: Story = { render: () => <ToolkitView /> };
