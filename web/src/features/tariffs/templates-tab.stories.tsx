import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoBuildingStates, demoTemplates } from './_fixture';
import { TemplatesTabView } from './templates-tab';

const meta = {
  title: 'Features/Tariffs/TemplatesTab',
  component: TemplatesTabView,
  args: {
    templates: demoTemplates,
    buildings: demoBuildingStates,
    canEdit: true,
    onCreate: () => {},
    onEdit: () => {},
    onDelete: () => {},
    onApply: () => {},
  },
} satisfies Meta<typeof TemplatesTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const ReadOnly: Story = { args: { canEdit: false } };
export const Empty: Story = { args: { templates: [] } };
