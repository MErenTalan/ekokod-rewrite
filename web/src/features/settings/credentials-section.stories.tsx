import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { Building, IntegrationCredential, IntegrationDefinition } from '@/lib/api/types';

import { CredentialsSectionView } from './credentials-section';

const credentials: IntegrationCredential[] = [
  { id: 'c-1', definition_id: 'd-1', provider: 'osos', subtype: 'Baskent', username: 'osos-user', has_secret: true, extra_keys: [], is_active: true, last_verified_at: '2026-09-15T09:00:00+03:00', updated_at: '2026-09-15T09:00:00+03:00' },
  { id: 'c-2', definition_id: 'd-4', provider: 'isolar', subtype: 'eu', has_secret: false, extra_keys: ['app_key'], is_active: true, last_verified_at: null, updated_at: '2026-09-15T09:00:00+03:00' },
];

const meta = {
  title: 'Features/Settings/CredentialsSection',
  component: CredentialsSectionView,
  args: {
    credentials,
    definitions: [] as IntegrationDefinition[],
    buildings: [] as Building[],
    canCreate: true,
    today: '2026-09-17',
    onSave: () => {},
    onVerify: () => {},
    onDiscover: () => {},
    onBackfill: () => {},
    onDelete: () => {},
    onConnectIsolar: () => {},
  },
} satisfies Meta<typeof CredentialsSectionView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const CompanyAdmin: Story = { args: { canCreate: false } };
export const Discovering: Story = {
  args: { job: { id: 'job-1', label: 'Analizörleri keşfet', status: 'running' } },
};
export const Empty: Story = { args: { credentials: [] } };
