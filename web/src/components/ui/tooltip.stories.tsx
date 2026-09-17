import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { Info } from 'lucide-react';
import { expect, userEvent, within } from 'storybook/test';

import { Tooltip } from './tooltip';

const meta = { title: 'UI/Tooltip', component: Tooltip, args: { content: '', children: <span /> } } satisfies Meta<typeof Tooltip>;
export default meta;
type Story = StoryObj<typeof meta>;

const trigger = (
  <button type="button" className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-foreground-muted type-small pointer-coarse:min-h-11">
    <Info aria-hidden className="size-4" />
    Ortalama tüketim
  </button>
);

export const Default: Story = {
  render: () => <Tooltip content="Seçili dönemdeki günlük ortalama">{trigger}</Tooltip>,
};

export const Open: Story = {
  tags: ['open'],
  render: () => (
    <div className="pt-16">
      <Tooltip content="Seçili dönemdeki günlük ortalama">{trigger}</Tooltip>
    </div>
  ),
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await expect(await within(canvasElement.ownerDocument.body).findByRole('tooltip')).toBeInTheDocument();
  },
};

export const LongTurkishLabel: Story = {
  render: () => (
    <Tooltip content="Reaktif Endüktif Tüketim Oranı Eşik Değeri aşıldığında dağıtım şirketi ceza uygular">
      {trigger}
    </Tooltip>
  ),
};
