import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { BillCalculator } from './bill-calculator';
import { initialValues } from './calculator-form';

const values = initialValues('2026-09-24');
const result = {
  basis: 'total' as const, days: 31, energy: '30000.00', distribution: '12000.00', power: '7500.00', overuse: '12743.00', vat_base: '62243.00',
  vat: '12448.60', total: '74691.60', vat_rate: '20.000', tariff: { effective_from: '2025-01-01', group_used: 'commercial_plus' },
};

const meta = {
  title: 'Features/Site/BillCalculator',
  component: BillCalculator,
  args: { values, errors: {}, result: null, pending: false, onChange: () => {}, onSubmit: () => {} },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof BillCalculator>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = {};
export const MediumVoltageResult: Story = { args: { values: { ...values, group: 'commercial', voltage: 'mv', term: 'binomial', total: '10000', contract: '100', demand: '120' }, result } };
export const Refused: Story = { args: { errors: { total: "Toplam tüketim T1, T2 ve T3'ün toplamına eşit olmalı." } } };
