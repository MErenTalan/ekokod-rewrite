import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Stepper } from './stepper';

const steps = [
  { id: 'file', label: 'Dosya yükle' },
  { id: 'map', label: 'Sütunları eşle', description: 'Başlıkları tarife alanlarıyla eşleştirin' },
  { id: 'review', label: 'Önizle ve uygula' },
];

const meta = { title: 'UI/Stepper', component: Stepper, args: { steps, currentIndex: 1 } } satisfies Meta<typeof Stepper>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const LongTurkishLabel: Story = { args: { steps: [{ id: 'a', label: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri' }, ...steps.slice(1)], currentIndex: 0 } };
