import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { fn } from 'storybook/test';

import { clauses, emptyProject, project } from './_fixture';
import { SummaryView } from './summary-view';

const meta = {
  title: 'Features/Iso50001/Summary',
  component: SummaryView,
  args: { project, clauses, today: '2026-06-15', canEdit: true, downloading: false, onDownload: fn(), onCalendar: fn() },
} satisfies Meta<typeof SummaryView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Project: Story = {};
export const NewProject: Story = { args: { project: emptyProject } };
export const Viewer: Story = { args: { canEdit: false } };
export const Downloading: Story = { args: { downloading: true } };
