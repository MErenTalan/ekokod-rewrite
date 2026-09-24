import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { fn } from 'storybook/test';

import { clauses } from './_fixture';
import { ChecklistView } from './checklist-view';

const meta = {
  title: 'Features/Iso50001/Checklist',
  component: ChecklistView,
  args: {
    clauses,
    templates: [{ id: 'significant-energy-uses', file_name: 'Onemli-Enerji-Kullanimlari.xlsx', clauses: ['6.3'], description: 'Pareto analizi.' }],
    onTemplate: fn(),
    renderSub: () => null,
  },
} satisfies Meta<typeof ChecklistView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Clauses: Story = {};
