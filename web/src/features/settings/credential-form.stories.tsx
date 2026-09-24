import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { Building, IntegrationDefinition } from '@/lib/api/types';

import { CredentialFormView, emptyDraft } from './credential-form';

const definitions = [
  { id: 'd-1', provider: 'osos', subtype: 'Baskent', endpoints: {}, updated_at: '' },
  { id: 'd-2', provider: 'gridbox', subtype: 'default', endpoints: {}, updated_at: '' },
  { id: 'd-3', provider: 'pm5340', subtype: 'default', endpoints: {}, updated_at: '' },
  { id: 'd-4', provider: 'isolar', subtype: 'eu', endpoints: {}, updated_at: '' },
] as IntegrationDefinition[];
const buildings = [
  { id: 'b-1', name: 'A1 Fabrika' },
  { id: 'b-2', name: 'A2 Depo' },
] as Building[];

const meta = {
  title: 'Features/Settings/CredentialForm',
  component: CredentialFormView,
  args: { definitions, buildings, value: emptyDraft('osos', 'Baskent'), onChange: () => {}, mode: 'create' as const },
} satisfies Meta<typeof CredentialFormView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Osos: Story = {};
export const Gridbox: Story = {
  args: { value: { ...emptyDraft('gridbox', 'default'), wiringNumbers: ['1001', '1002'], buildingId: 'b-1' } },
};
export const Pm5340: Story = { args: { value: emptyDraft('pm5340', 'default') } };
export const Isolar: Story = { args: { value: emptyDraft('isolar', 'eu') } };
