import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { expect, userEvent, within } from 'storybook/test';

import { ExportMenu } from './export-menu';

const meta = { title: 'Domain/ExportMenu', component: ExportMenu, args: { onExport: () => {} } } satisfies Meta<typeof ExportMenu>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Open: Story = {
  tags: ['open', 'modal-popup'],
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole('button'));
    await expect(await within(canvasElement.ownerDocument.body).findByRole('menu')).toBeInTheDocument();
  },
};
export const Busy: Story = { args: { busyFormat: 'excel' } };
export const Disabled: Story = { args: { disabled: true } };
