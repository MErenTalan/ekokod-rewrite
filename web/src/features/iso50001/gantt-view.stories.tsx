import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { clauses, emptyProject, project } from './_fixture';
import { GanttView } from './gantt-view';

const meta = { title: 'Features/Iso50001/Gantt', component: GanttView, args: { project, clauses, today: '2026-06-15' } } satisfies Meta<typeof GanttView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Planned: Story = {};
export const NotEnoughDates: Story = { args: { project: emptyProject } };
