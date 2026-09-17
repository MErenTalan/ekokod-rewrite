import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { FileText } from 'lucide-react';

import { Button } from './button';
import { EmptyState } from './empty-state';

const meta = {
  title: 'UI/EmptyState',
  component: EmptyState,
  args: { title: 'Bu dönem için fatura yok', description: 'Binaya bir tarife atayın, ardından dönemi hesaplayın.' },
} satisfies Meta<typeof EmptyState>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { icon: FileText, action: <Button size="sm">Tarife ata</Button> } };
export const LongTurkishLabel: Story = { args: { title: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri tanımlanmamış' } };
