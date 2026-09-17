import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { PlantDevice } from '@/lib/api/types';

import { PlantFormView, emptyPlant } from './plant-form';

const devices = [
  { id: 'd-1', device_sn: 'INV-1', device_name: 'İnvertör 1', brand: 'Sungrow', model: 'SG50', rated_power_kw: '50', status: 'ok' },
] as PlantDevice[];

const filled = {
  ...emptyPlant(),
  id: 'p-1',
  name: 'Çatı GES',
  monthlyTargets: Array.from({ length: 12 }, (_, i) => String(1000 + i * 50)),
  alarmRecipients: ['ges@ornek.com.tr'],
};

const meta = {
  title: 'Features/Settings/PlantForm',
  component: PlantFormView,
  args: { value: filled, onChange: () => {}, devices },
} satisfies Meta<typeof PlantFormView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Existing: Story = {};
export const NewPlant: Story = { args: { value: emptyPlant(), devices: [] } };
