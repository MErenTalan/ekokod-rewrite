import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoTariff } from './_fixture';
import { draftFrom, emptyDraft } from './tariff-draft';
import { TariffFormView } from './tariff-form';

const fixed = {
  ...emptyDraft(),
  effectiveFrom: '2026-09-01',
  singleTimePrice: '3.150000',
  distributionCost: '0.850000',
  reactivePowerPrice: '1.200000',
  vatRate: '20',
  taxes: [{ name: 'BTV', rate: '5' }],
};

const meta = {
  title: 'Features/Tariffs/TariffForm',
  component: TariffFormView,
  args: { draft: fixed, onDraftChange: () => {}, onSubmit: () => {} },
} satisfies Meta<typeof TariffFormView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const FixedPrice: Story = {};
/** The KBK fieldset only exists in this mode (R249); the currency says TRY-only (R127). */
export const PtfYekdem: Story = { args: { draft: draftFrom(demoTariff) } };
export const WithServerErrors: Story = {
  args: { draft: { ...fixed, singleTimePrice: '' }, serverErrors: { single_time_price: 'Zorunlu alan' } },
};
export const ReadOnly: Story = { args: { readOnly: true } };
