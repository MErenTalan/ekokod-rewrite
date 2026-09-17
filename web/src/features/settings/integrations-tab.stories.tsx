import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import type { IntegrationDefinition } from '@/lib/api/types';

import { IntegrationsTabView } from './integrations-tab';

const definitions: IntegrationDefinition[] = [
  { id: 'd-1', provider: 'osos', subtype: 'Baskent', endpoints: { authentication: 'https://api.ornek/auth', analyzer_list: 'https://api.ornek/list' }, updated_at: '2026-09-01T09:00:00+03:00' },
  { id: 'd-2', provider: 'gridbox', subtype: 'default', endpoints: { token: 'https://gridbox.ornek/token' }, updated_at: '2026-09-02T09:00:00+03:00' },
];

const meta = {
  title: 'Features/Settings/IntegrationsTab',
  component: IntegrationsTabView,
  args: { definitions, onSave: () => {}, onDelete: () => {} },
} satisfies Meta<typeof IntegrationsTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { definitions: [] } };
