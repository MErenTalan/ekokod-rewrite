import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoAnalyzers } from './_fixture';
import { GenerateCardView } from './generate-card';

const meta = {
  title: 'Features/Bills/GenerateCard',
  component: GenerateCardView,
  args: { analyzers: demoAnalyzers, period: '2026-08', buildingId: 'b-1', onGenerate: () => {}, onDownload: () => {} },
} satisfies Meta<typeof GenerateCardView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Running: Story = { args: { running: true } };
/** Every §7.10 data failure reaches the operator as its own sentence (R238). */
export const WithFailure: Story = { args: { failure: 'noTariffForBuilding' } };
export const Ready: Story = { args: { readyBillIds: ['bill-9'] } };
