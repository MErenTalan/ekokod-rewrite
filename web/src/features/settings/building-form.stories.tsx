import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { User } from '@/lib/api/types';

import { BuildingFormView, emptyBuilding } from './building-form';

const users = [
  { id: 'u-1', name: 'Burak Şahin', email: 'burak@ornek.com.tr' },
  { id: 'u-2', name: 'Elif Arslan', email: 'elif@ornek.com.tr' },
] as User[];

const meta = {
  title: 'Features/Settings/BuildingForm',
  component: BuildingFormView,
  args: { value: emptyBuilding(), onChange: () => {}, users, tariffHistory: [] },
} satisfies Meta<typeof BuildingFormView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const NewBuilding: Story = {};
export const WithTariffHistory: Story = {
  args: {
    value: { ...emptyBuilding(), id: 'b-1', name: 'A1 Fabrika', contacts: [{ name: 'Ayşe Kaya', phone: '0532 000 00 00' }] },
    tariffHistory: [
      { id: 't-1', effective_from: '2026-01-01', name: 'Sanayi OG' },
      { id: 't-2', effective_from: '2025-01-01', name: 'Sanayi OG (2025)' },
    ],
  },
};
